package handler

import (
	"log"
	"net/http"
	"strings"
	"time"

	"askflow/internal/conversation"
	"askflow/internal/errlog"
	"askflow/internal/llm"
	"askflow/internal/query"
)

// HandleQuery processes a user question through the RAG pipeline.
func HandleQuery(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		// Validate user session
		sessionID, err := GetUserSession(app, r)
		if err != nil {
			WriteError(w, http.StatusUnauthorized, err.Error())
			return
		}
		var req query.QueryRequest
		if err := ReadJSONBody(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		question := strings.TrimSpace(req.Question)
		if question == "" {
			WriteError(w, http.StatusBadRequest, "question is required")
			return
		}
		// Limit question length to prevent abuse
		if len(question) > 10000 {
			WriteError(w, http.StatusBadRequest, "question too long (max 10000 characters)")
			return
		}
		req.Question = question
		// Validate product_id format if provided
		if req.ProductID != "" && !IsValidOptionalID(req.ProductID) {
			WriteError(w, http.StatusBadRequest, "invalid product_id")
			return
		}
		// Default to first product if no product_id specified
		if req.ProductID == "" {
			firstID, pErr := app.GetFirstProductID()
			if pErr == nil && firstID != "" {
				req.ProductID = firstID
			}
		}
		// Shop owner isolation: force product_id to shop's module product ID.
		if pid, err := resolveShopListProductID(r, req.ProductID); err != nil {
			WriteError(w, http.StatusForbidden, err.Error())
			return
		} else {
			req.ProductID = pid
		}

		// Conversation history handling
		cfg := app.configManager.Get()
		if cfg == nil {
			log.Printf("[Query] WARNING: config is nil, conversation will be disabled")
		} else {
			log.Printf("[Query] config loaded: conversation.enabled=%v", cfg.Conversation.Enabled)
		}
		
		// Check both system-level and product-level conversation settings
		conversationEnabled := cfg != nil && cfg.Conversation.Enabled
		if conversationEnabled && req.ProductID != "" {
			// Check product-level conversation setting
			product, err := app.productService.GetByID(req.ProductID)
			if err != nil {
				// Log error but don't fail the request - default to disabled for safety
				log.Printf("[Query] WARNING: failed to get product %s for conversation check: %v (defaulting to disabled)", req.ProductID, err)
				conversationEnabled = false
			} else if product != nil {
				if !product.ConversationEnabled {
					log.Printf("[Query] conversation disabled for product %s (product setting)", req.ProductID)
					conversationEnabled = false
				} else {
					log.Printf("[Query] conversation enabled for product %s", req.ProductID)
				}
			} else {
				// Product not found - should not happen but handle gracefully
				log.Printf("[Query] WARNING: product %s not found for conversation check (defaulting to disabled)", req.ProductID)
				conversationEnabled = false
			}
		}
		
		var conversationID string
		var history []llm.HistoryMessage

		if conversationEnabled {
			// Get anonymous ID from header or request body
			anonymousID := r.Header.Get("X-Anonymous-ID")
			if anonymousID == "" {
				anonymousID = req.AnonymousID
			}

			// Determine userID - each session is treated as a separate user to prevent context pollution
			// For anonymous users: use session ID as the unique identifier
			// For logged-in users: use actual user ID
			userID := req.UserID
			if userID == "" && sessionID != "" {
				// Anonymous user: session ID is the unique identifier
				userID = "session:" + sessionID
			} else if userID == "" && anonymousID != "" {
				// Fallback: use anonymous ID if no session
				userID = "anon:" + anonymousID
			}

			convMgr := conversation.NewManager(app.readDB, app.db)

			log.Printf("[Query] userID=%s, sessionID=%s, productID=%s", userID, sessionID, req.ProductID)

			// Handle conversation ID
			if req.ConversationID != "" {
				log.Printf("[Query] frontend provided conversation_id=%s", req.ConversationID)
				// Validate that the conversation belongs to this user
				valid, _ := convMgr.ValidateConversation(req.ConversationID, userID)
				if valid {
					conversationID = req.ConversationID
					log.Printf("[Query] validated conversation_id=%s", conversationID)
				} else {
					// Invalid conversation_id, create new one
					log.Printf("[Query] invalid conversation_id %s for user %s, creating new", req.ConversationID, userID)
					conversationID, _ = convMgr.GetOrCreateConversation(userID, sessionID, req.ProductID)
				}
			} else {
				log.Printf("[Query] no conversation_id from frontend, checking for recent conversation")
				if userID != "" {
					// No conversation_id provided, try to auto-link to recent conversation
					// This is a temporary workaround for frontend not passing conversation_id
					recentConvID, _ := convMgr.GetMostRecentConversation(userID, req.ProductID, 5*time.Minute)
					if recentConvID != "" {
						conversationID = recentConvID
						log.Printf("[Query] auto-linked to recent conversation %s (age < 5min)", conversationID)
					} else {
						// No recent conversation, create new one
						conversationID, _ = convMgr.GetOrCreateConversation(userID, sessionID, req.ProductID)
						log.Printf("[Query] created new conversation %s", conversationID)
					}
				}
			}

			// Load conversation history
			if conversationID != "" {
				maxHistory := cfg.Conversation.MaxHistoryMessages
				if maxHistory <= 0 {
					maxHistory = 10
				}
				messages, err := convMgr.GetRecentMessages(conversationID, maxHistory)
				if err != nil {
					log.Printf("[Query] failed to load conversation history: %v", err)
				} else {
					for _, msg := range messages {
						history = append(history, llm.HistoryMessage{
							Role:    msg.Role,
							Content: msg.Content,
						})
					}
					log.Printf("[Query] loaded %d history messages for conversation %s", len(history), conversationID)
					if len(history) > 0 {
						previewLen := 50
						if len(history[0].Content) < previewLen {
							previewLen = len(history[0].Content)
						}
						log.Printf("[Query] history preview: first message role=%s, content=%s", history[0].Role, history[0].Content[:previewLen])
					}
				}
			} else {
				log.Printf("[Query] no conversationID, history will be empty")
			}
		} else {
			log.Printf("[Query] conversation disabled, history will be empty")
		}

		resp, err := app.queryEngine.QueryWithHistory(req, history)
		if err != nil {
			log.Printf("[Query] error: %v", err)
			errlog.Logf("[Query] query processing failed: %v", err)
			WriteError(w, http.StatusInternalServerError, "查询处理失败，请稍后重试")
			return
		}

		// Save conversation messages
		if conversationEnabled && conversationID != "" {
			convMgr := conversation.NewManager(app.readDB, app.db)
			// Save user question
			if err := convMgr.AddMessage(conversationID, "user", req.Question, req.ImageData, nil); err != nil {
				log.Printf("[Query] WARNING: failed to save user message: %v", err)
			} else {
				log.Printf("[Query] saved user message to conversation %s", conversationID)
			}
			// Save assistant answer
			sources := make([]conversation.SourceRef, len(resp.Sources))
			for i, s := range resp.Sources {
				sources[i] = conversation.SourceRef{
					DocumentName: s.DocumentName,
					ChunkIndex:   s.ChunkIndex,
				}
			}
			if err := convMgr.AddMessage(conversationID, "assistant", resp.Answer, "", sources); err != nil {
				log.Printf("[Query] WARNING: failed to save assistant message: %v", err)
			} else {
				log.Printf("[Query] saved assistant message to conversation %s", conversationID)
			}
			resp.ConversationID = conversationID
			log.Printf("[Query] returning conversation_id=%s to frontend", conversationID)
		}

		// Record question for FAQ weight tracking (async, non-blocking)
		go app.RecordFAQ(req.ProductID, req.Question)
		// Strip debug info for non-admin users to prevent information leakage
		// TEMPORARY: Debug mode enabled for all users during development
		// TODO: Re-enable admin check in production
		/*
		if resp.DebugInfo != nil {
			_, _, adminErr := GetAdminSession(app, r)
			if adminErr != nil {
				resp.DebugInfo = nil
			}
		}
		*/
		// Check if product allows document download
		if req.ProductID != "" {
			p, pErr := app.GetProduct(req.ProductID)
			if pErr == nil && p != nil {
				resp.AllowDownload = p.AllowDownload
			}
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}
