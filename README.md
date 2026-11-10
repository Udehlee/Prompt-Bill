# Prompt-Bill
Prompt-Bill is a lightweight, high-performance reverse proxy that sits between client applications and LLM providers such as Groq, OpenAI, and Google Gemini.

It authenticates incoming requests, forwards them to upstream providers, streams responses using Server-Sent Events (SSE) and records usage metrics in PostgreSQL.

It provides multi-tenant usage tracking, allowing customers to monitor token consumption, request activity, latency, and AI spending through a dashboard.

![Prompt-Bill Dashboard](docs/images/metrics.png)

## Features
- **Streaming proxy:** Streams SSE responses without buffering the entire response body (`FlushInterval: -1`).
- **Multi-tenant API key authentication:** Authenticates customers using API keys. Only SHA-256 key hashes are stored.
- **Automatic provider failover:** Supports fallback to another provider on `429`, `500`, `502`, `503`, `504`, or connection resets.
- **Deterministic pricing:** Uses integer fixed-point arithmetic to calculate costs in microdollars, avoiding floating-point rounding errors.
- **Usage dashboard:** Provides per-tenant analytics, date presets, and keyset-paginated request logs.
- **Privacy-focused tracking:** Prompts, completions, and upstream API keys are not stored or logged. Only usage metrics and attribution metadata are persisted.

### Pricing Calculation
Costs are calculated in microdollars:
```text
cost = floor(prompt_tokens × prompt_rate / 10^6)
     + floor(completion_tokens × completion_rate / 10^6)
```
Where:

`1 USD = 1,000,000 microdollars`

## Architecture
Prompt-Bill uses API key authentication to identify tenants and associate requests with their usage records.

### Request Flow
```mermaid
sequenceDiagram
    participant Customer
    participant Proxy as Prompt-Bill
    participant DB as PostgreSQL
    participant LLM as Upstream Provider

    Customer->>Proxy: Send request with tracker API key
    Proxy->>DB: Verify key and resolve tenant
    Proxy->>LLM: Forward request
    LLM-->>Proxy: Stream response using SSE
    Proxy-->>Customer: Stream response
    Proxy->>DB: Record usage, latency, and cost
```

The dashboard uses the authenticated tenant to retrieve its usage data from PostgreSQL.

## Requirements
- Go 
- Docker and Docker Compose
- An upstream provider API key (e.g. Groq or OpenAI)

## Installation
- Clone the repository.
  ```bash
  git clone https://github.com/Udehlee/Prompt-Bill.git
    cd gyara
    pnpm install

   ```

- Create your .env file by copying the example file:
    cp .env.example .env
    Open .env and provide your upstream provider API key and any other required environment values.

-  Start the services:
   ```bash
   docker compose up -d --build
   ```

   This starts two containers:
   - `prompt-bill`: The Go reverse proxy and dashboard on port `8080`.
   - `prompt-bill-postgres`: PostgreSQL 17 on port `5432`, with the required schema and pricing tables applied.

## Usage
### Provision a Tenant API Key
Generate a unique API key for a customer:
```bash
docker exec prompt-bill /app/tracker -provision-key -tenant "Customer Org" -key-name "Production Key"
```
The command prints a `trk_live_...` key. It is displayed only once, so store it securely.
Only the SHA-256 hash of the key is stored in the database.

### Send Requests Through the Proxy
Point any OpenAI-compatible SDK to Prompt-Bill:
```Go
    client := openai.NewClient(
        option.WithBaseURL("https://your-domain.com/v1"),
        option.WithAPIKey("trk_live_xxxxxxxx_your_issued_key"),
        option.WithHTTPHeader("X-Tracker-User-Id", "customer_user_123"),
        option.WithHTTPHeader("X-Tracker-Feature", "support_chat"),
    )
```
`X-Tracker-User-Id` and `X-Tracker-Feature` are optional headers used to attribute spending to specific users and features.

Prompt-Bill authenticates the tracker API key and uses its configured upstream provider credentials when forwarding requests.

### View the Dashboard
Open [http://localhost:8080/dashboard](http://localhost:8080/dashboard), enter your `trk_live_...` key, and click **Connect**.

The dashboard provides:
- Total spending and token consumption
- Daily spending trends
- Usage grouped by model
- Top-spending users and features
- Request history and latency metrics
- Date filters, including Today, 7D, and 30D

Each key can access only its own tenant's usage data.

## Demo
Prompt-Bill includes three pre-seeded demo tenants for exploring the dashboard locally.

| Tenant | Demo API Key | Example Use Case |
|---|---|---|
| Acme AI Labs | `trk_live_af6b5e01_bHuQRxFRC2OToyKBI_Nv_TDkoWTE8T8z` | E-commerce chatbot and shopping assistant |
| FinTech Global | `trk_live_91ebe5be_z2JieK4nqXNYgH2fCt1Uk6Hv4hD8RlKu` | Fraud detection and portfolio analysis |
| HealthTech Systems | `trk_live_0ba44580_GOdTGCIpZn8t-09Y1gP7MmEBSwXMaX42` | Clinical triage and doctor copilot |

Example user identifiers:

- Acme AI Labs: `alice`, `bob`
- FinTech Global: `marcus`, `elena`
- HealthTech Systems: `dr_sarah`, `dr_chen`

To explore the demo:

1. Start the application.
2. Open `http://localhost:8080/dashboard`.
3. Enter one of the demo API keys.
4. Click **Connect**.
5. Select a date range to explore that tenant's analytics.

Each demo key displays separate usage records and spending data.

> **Note:** Demo keys are for local demonstration only. Do not use them for production traffic.

## API Reference
All reporting endpoints except `/readyz` require:
```http
Authorization: Bearer trk_live_your_api_key
```

| Endpoint | Method | Description |
|---|---|---|
| `/readyz` | GET | Health check, including PostgreSQL connectivity |
| `/api/v1/summary` | GET | Total spend, token volume, and P95 latency |
| `/api/v1/daily-costs` | GET | Daily spending trends |
| `/api/v1/top-spenders` | GET | Spending ranked by feature (`?by=feature`) or user (`?by=user`) |
| `/api/v1/models` | GET | Token volume and spending grouped by model |
| `/api/v1/requests` | GET | Keyset-paginated request logs |

The main proxy endpoint is:
```http
POST /v1/chat/completions
```

## Testing and Benchmarks
Run all Go tests:

```bash
go test -v ./...
```
- Use these commands to check your docker services
```bash
docker compose ps                        
docker compose logs -f prompt-bill       
docker compose down                       
docker compose down -v                    
```
