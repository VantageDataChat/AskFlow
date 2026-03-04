package handler

import (
	"log"
	"net/http"
	"strings"

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
		conversationEnabled := cfg != nil && cfg.Conversation.Enabled
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

			// Handle conversation ID
			if req.ConversationID != "" {
				// Validate that the conversation belongs to this user
				valid, _ := convMgr.ValidateConversation(req.ConversationID, userID)
				if valid {
					conversationID = req.ConversationID
				} else {
					// Invalid conversation_id, create new one
					log.Printf("[Query] invalid conversation_id %s for user %s, creating new", req.ConversationID, userID)
					conversationID, _ = convMgr.GetOrCreateConversation(userID, sessionID, req.ProductID)
				}
			} else if userID != "" {
				// No conversation_id provided, create new conversation
				conversationID, _ = convMgr.GetOrCreateConversation(userID, sessionID, req.ProductID)
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
				}
			}
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
			_ = convMgr.AddMessage(conversationID, "user", req.Question, req.ImageData, nil)
			// Save assistant answer
			sources := make([]conversation.SourceRef, len(resp.Sources))
			for i, s := range resp.Sources {
				sources[i] = conversation.SourceRef{
					DocumentName: s.DocumentName,
					ChunkIndex:   s.ChunkIndex,
				}
			}
			_ = convMgr.AddMessage(conversationID, "assistant", resp.Answer, "", sources)
			resp.ConversationID = conversationID
		}

		// Record question for FAQ weight tracking (async, non-blocking)
		go app.RecordFAQ(req.ProductID, req.Question)
		// Strip debug info for non-admin users to prevent information leakage
		if resp.DebugInfo != nil {
			_, _, adminErr := GetAdminSession(app, r)
			if adminErr != nil {
				resp.DebugInfo = nil
			}
		}
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
