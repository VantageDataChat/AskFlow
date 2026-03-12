## Technical Context: Eino-based Page-Index RAG System

### 1. System Philosophy

This system evolves from **Naive RAG** (vector-only) to **Structural RAG**. Instead of arbitrary text chunking, it uses **Page-level Indexing** as the primary retrieval and context unit to maintain document hierarchy and logical continuity, specifically for product manuals and knowledge bases.

### 2. Core Components & Data Schema

- **Parser (Go + gopdf2):**
  - **Logic:** Extracts text fragments with $(X, Y)$ coordinates and $FontSize$.
  - **Heuristics:** Groups fragments by $Y$-coordinate proximity into blocks. Detects headers via $FontSize$ outliers.
  - **Output:** `PageNode` struct containing `MarkdownContent` (with reconstructed tables), `PageNum`, and `HeaderPath`.
- **Storage Strategy:**
  - **Vector DB (e.g., Qdrant):** Stores small sub-page chunks for high-recall semantic search.
  - **KV Store (e.g., Redis):** Stores the full `PageNode` keyed by `page_id`.
- **Orchestration (Eino Framework):**
  - Uses **Eino Graph** to manage the lifecycle: `QueryTransform -> HybridRetriever -> PageExpander -> Reranker -> Generator`.

### 3. Implementation Logic for AI Agent

When generating code, follow these **Eino-specific** patterns:

- **Type Safety:** Use Go generics for `eino.Graph` input/output. Define custom types for `PageContext` to pass metadata (PageNum, DocName) through the graph.
- **Node Composition:**
  - Implement the **"Small-to-Big"** pattern: The `Retriever` fetches chunks; a subsequent `LambdaNode` (the Expander) must use the `page_id` from metadata to fetch the full `PageNode` from the KV store.
  - Inject a **Reranker** node after expansion to ensure the full page context is relevant to the user query.
- **Functional Requirements:**
  - All generated prompts must include instructions for the LLM to cite sources as `[Doc Name, Page X]`.
  - Handle PDF tables by converting them into standard Markdown table format during the parsing stage.

### 4. Technical Stack Constraints

- **Language:** Golang 1.22+
- **Framework:** Eino ([github.com/cloudwego/eino](https://github.com/cloudwego/eino))
- **PDF Library:** `gopdf2` (vantagics/gopdf2)
- **Vector DB:** Qdrant (preferred for its strong Payload filtering)