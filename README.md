# Mantis Skill

A declarative YAML-based tool definition framework for building AI agent capabilities. Define what your tools do — not how to wire them — and let the engine handle execution, input resolution, variable replacement, output formatting, and workflow orchestration.

Mantis Skill powers production AI agents that handle thousands of conversations daily and hundreds of thousands of tool calls, with a rich feature set covering API calls, database queries, terminal commands, code execution, browser automation, and more - in a reliable manner. 

## Why Mantis Skill?

Building AI agent tools usually means writing boilerplate: parse inputs, call APIs, format outputs, handle errors, chain results, manage retries. Mantis Skill replaces all of that with a YAML definition:

```yaml
tools:
  - name: weather
    description: Weather information service
    functions:
      - name: GetForecast
        operation: api_call
        description: Get weather forecast for a city
        input:
          - name: city
            description: City name
            origin: inference
            successCriteria: A valid city name
        steps:
          - method: GET
            url: "https://api.weather.com/forecast?city=$city"
        output:
          description: Weather forecast
          value: "$result[1].forecast"
```

No code. No HTTP client setup. No input parsing. The engine handles it.

## Features

### Declarative Tool Protocol (`skill/`)

The complete YAML tool definition language with 350+ test cases covering:

- **12 operation types**: `api_call`, `db`, `terminal`, `code`, `web_browse`, `mcp`, `format`, `pdf`, `gdrive`, `desktop_use`, `policy`, `initiate_workflow`
- **6 input origins**: `inference` (AI-extracted), `chat`, `function` (from another tool), `knowledge`, `search`, `memory`
- **Rich control flow**: `runOnlyIf` conditions, `forEach` loops with `breakIf`, `reRunIf` post-execution checks, `needs` dependencies
- **Callbacks**: `onSuccess`, `onFailure`, `onSkip`, `onMissingUserInfo`, `onUserConfirmationRequest` with parameter passing and forEach iteration
- **Output formatting**: structured objects, lists, field extraction with `$result[step][index].field` navigation
- **Triggers**: `time_based` (cron), `always_on_user_message`, `always_on_team_message`, `flex_for_user`, email variants, meeting events
- **i18n**: Localized strings (`en`/`pt`/`es`) for descriptions, labels, and user-facing messages
- **Validation**: 8,400-line parser with compile-time validation, variable reference checking, orphan function detection, dependency cycle detection, trigger reachability analysis
- **KPI definitions**: Declarative metrics with aggregation (`count`, `sum`, `avg`, `ratio`), comparison periods, and materialization
- **Database migrations**: Versioned schema migrations tracked per-tool
- **Sanitization**: Prompt injection protection via fence nonces, strict mode, or LLM extraction
- **Caching**: Per-function result caching with configurable TTL

### Execution Engine (`engine/`)

The runtime that turns YAML definitions into executable tools:

- **Variable replacement**: System variables (`$USER`, `$ADMIN`, `$COMPANY`, `$ME`, `$UUID`, `$DATETIME`), environment variables, input references, step result navigation, loop context variables
- **Multi-step execution**: Sequential step execution with result accumulation, conditional skipping, forEach iteration
- **Input fulfillment**: AI-powered input extraction from conversation context, with fallback chains and error recovery
- **Output formatting**: Raw results transformed into structured objects, lists, or formatted strings
- **Workflow orchestration**: Interfaces for agentic coordinators, checkpoint/resume, workflow event tracking
- **Trigger system**: Cron scheduling, message-based triggers, meeting event triggers with concurrent execution control

### Provider Architecture (`types/`)

Clean interfaces for every external dependency. Bring your own:

| Provider | Purpose |
|---|---|
| `LLMProvider` | AI inference (input extraction, formatting, conditions) |
| `DatabaseProvider` | SQL execution for `db` operations |
| `HTTPClient` | HTTP requests for `api_call` operations |
| `TerminalProvider` | Shell commands for `terminal` operations |
| `CodeExecutor` | Claude Code / agent SDK for `code` operations |
| `BrowserProvider` | Browser automation for `web_browse` operations |
| `MCPProvider` | Model Context Protocol tool execution |
| `PDFProvider` | PDF document generation |
| `GDriveProvider` | Google Drive file operations |
| `AuthProvider` | Authentication headers for API calls |
| `FileProvider` | Temporary file management |
| `CacheProvider` | Function result caching |
| `MessageSender` | User/team message delivery |
| `UserRepository` | User data access |
| `ToolRepository` | Function execution tracking |
| `WorkflowRepository` | Workflow persistence |
| `CheckpointRepository` | Pause/resume state management |

## Installation

```bash
go get github.com/bbiangul/mantis-skill
```

Requires Go 1.25+.

## Quick Start

### Parse and Validate a Tool Definition

```go
package main

import (
    "fmt"
    "github.com/bbiangul/mantis-skill/skill"
)

func main() {
    yaml := `
tools:
  - name: crm
    description: CRM operations
    functions:
      - name: CreateContact
        operation: db
        description: Create a new contact in the CRM
        input:
          - name: contactName
            description: Full name of the contact
            origin: inference
            successCriteria: A person's full name
          - name: email
            description: Contact email address
            origin: inference
            successCriteria: A valid email address
            regex: "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"
        steps:
          - query: |
              INSERT INTO contacts (name, email, created_at)
              VALUES ($contactName, $email, $DATETIME)
        output:
          description: Confirmation message
          value: "Contact $contactName created successfully"
`

    customTool, errors := skill.ParseAndValidateYAML([]byte(yaml))
    if len(errors) > 0 {
        for _, err := range errors {
            fmt.Printf("Validation error: %s\n", err)
        }
        return
    }

    fmt.Printf("Tool: %s (%d functions)\n",
        customTool.Tools[0].Name,
        len(customTool.Tools[0].Functions))
}
```

### Wire the Engine with Providers

```go
package main

import (
    "context"
    "github.com/bbiangul/mantis-skill/engine"
    "github.com/bbiangul/mantis-skill/types"
)

func main() {
    // Create your provider implementations
    config := types.Config{
        LLM:      myLLMProvider,       // Required: AI inference
        Database:  myDatabaseProvider,  // Required: SQL execution
        Logger:    myLogger,           // Required: structured logging

        // Optional — enable only the operations you need
        HTTP:     myHTTPClient,
        Terminal: myTerminalProvider,
        Code:     myCodeExecutor,
    }

    // Set the logger for the engine package
    engine.SetLogger(myLogger)

    // The engine uses these providers when executing tool operations
    _ = config
}
```

## YAML Tool Definition Reference

### Tool Structure

```yaml
version: "1.0"
env:
  - name: API_KEY
    value: "${API_KEY}"
    description: API key for external service

tools:
  - name: my_tool
    description: What this tool does
    functions:
      - name: MyFunction
        operation: api_call    # Operation type
        description: What this function does
        triggers:              # When to execute
          - type: flex_for_user
        input:                 # Input parameters
          - name: param1
            description: Parameter description
            origin: inference
            successCriteria: What makes a valid value
        steps:                 # Execution steps
          - method: GET
            url: "https://api.example.com/$param1"
        output:                # Output formatting
          description: What the output contains
          value: "$result[1].data"
```

### Operation Types

| Operation | Description | Key Step Fields |
|---|---|---|
| `api_call` | HTTP API calls | `method`, `url`, `body`, `headers`, `saveAsFile` |
| `db` | SQL database queries | `query` (SELECT/INSERT/UPDATE/DELETE/UPSERT) |
| `terminal` | Shell command execution | `command`, `timeout` |
| `code` | Claude Code / AI agent SDK | `prompt`, `with.allowedTools`, `with.model` |
| `web_browse` | Browser automation | `open_url`, `extract_content`, `click`, `type`, `screenshot` |
| `mcp` | Model Context Protocol | `mcp.stdio` or `mcp.sse` config, `tool_name`, `params` |
| `format` | AI-powered text formatting | Inference-origin inputs, no steps needed |
| `pdf` | PDF document generation | `pdf.content` blocks, `pdf.pageSize`, `pdf.orientation` |
| `gdrive` | Google Drive operations | `method` (list/upload/download), `query`, `fileId` |
| `policy` | Rule evaluation | `rules` with conditions and actions |
| `initiate_workflow` | Start a sub-workflow | `start_workflow` steps with `action` references |
| `desktop_use` | Desktop automation | `screenshot`, `click`, `type`, `scroll` |

### Input Origins

```yaml
input:
  # AI extracts from conversation
  - name: city
    origin: inference
    successCriteria: A valid city name
    regex: "^[A-Z][a-z]+"          # Optional validation

  # Value comes from another function's output
  - name: userId
    origin: function
    value: "lookupUser"            # Function name to call

  # Extracted from conversation history
  - name: preference
    origin: chat
    successCriteria: User's preference

  # Hardcoded or variable reference
  - name: timestamp
    value: "$DATETIME"
```

### Control Flow

```yaml
functions:
  - name: ConditionalFunction
    operation: db
    runOnlyIf:
      deterministic: "$channel == 'whatsapp'"
    steps:
      - query: "SELECT * FROM users WHERE id = $userId"
        runOnlyIf:
          deterministic: "$userId != ''"
        forEach:
          items: "$userIds"
          itemVar: "userId"
          breakIf: "$item.status == 'found'"
    reRunIf:
      deterministic: "$result[1].length == 0"
      maxRetries: 3
```

### Callbacks

```yaml
functions:
  - name: ProcessOrder
    operation: api_call
    steps:
      - method: POST
        url: "https://api.orders.com/create"
        body: '{"item": "$item", "qty": $qty}'
    onSuccess:
      - name: SendConfirmation
        params:
          orderId: "$result[1].id"
        forEach:
          items: "$result[1].notifications"
          itemVar: "notification"
    onFailure:
      - name: NotifySupport
        params:
          error: "$error"
    onSkip:
      - name: LogSkipped
```

### System Variables

| Variable | Description |
|---|---|
| `$USER.firstname`, `$USER.email`, etc. | Current user information |
| `$ADMIN.name`, `$ADMIN.email` | Admin/owner information |
| `$COMPANY.name`, `$COMPANY.id` | Company information |
| `$ME.name` | AI assistant identity |
| `$UUID` | Random UUID |
| `$DATETIME` | Current ISO timestamp |
| `$DATE` | Current date (YYYY-MM-DD) |
| `$TIME` | Current time (HH:MM:SS) |
| `$TIMESTAMP` | Unix timestamp |
| `$CHANNEL` | Communication channel |
| `$MESSAGE` | Current message text |
| `$TEMP_DIR` | Temporary directory path |
| `$FILE.url`, `$FILE.mimetype` | Attached file information |

## Project Structure

```
mantis-skill/
├── skill/              # YAML tool protocol — parsing, validation, models
│   ├── parser.go       # 8,400-line validator with 350+ tests
│   ├── models.go       # Tool, Function, Step, Input, Output structs
│   ├── const.go        # Operation types, triggers, data origins
│   └── helpers.go      # Deterministic expression evaluator, coalesce
├── engine/             # Execution engine — runs parsed tools
│   ├── yaml_defined_tool.go          # Core executor (~10K lines)
│   ├── variable_replacer.go          # $variable resolution
│   ├── input_fulfiller.go            # AI-powered input extraction
│   ├── tool_engine.go                # Tool registry and lifecycle
│   ├── tool_functions.go             # System functions (ask, learn, search)
│   ├── triggers.go                   # Cron + message trigger system
│   ├── output_formatter.go           # Result formatting
│   ├── agentic_inference.go          # Multi-step AI reasoning
│   ├── workflow_manager.go           # Workflow state machine
│   ├── models/                       # Interfaces + state management
│   │   ├── interfaces.go             # IToolEngine, IVariableReplacer, etc.
│   │   ├── coordinator_state.go      # Agentic coordinator state
│   │   └── workflow_event_models.go  # Event cache for observability
│   ├── sanitizer/                    # Prompt injection protection
│   └── prompts/                      # System prompts for coordinator
└── types/              # Shared types + provider interfaces
    └── types.go        # Message, User, Config, all provider interfaces
```

## Status

Mantis Skill is in **early release**. The YAML tool protocol (`skill/`) is production-hardened with 350+ tests and has been running in production for over a year. The execution engine (`engine/`) is functional but some operations require host-provided implementations via the provider interfaces.

| Layer | Maturity |
|---|---|
| YAML parsing & validation | Production-ready |
| Variable replacement | Production-ready |
| DB, Terminal, Format operations | Production-ready |
| API call operations | Requires `HTTPClient` provider |
| Code operations | Requires `CodeExecutor` provider |
| MCP operations | Requires `MCPProvider` provider |
| Web browse, Desktop, PDF, GDrive | Requires respective providers |
| Input fulfillment | Functional (advanced flows require `LLMProvider`) |
| Workflow orchestration | Interface-only (`IAgenticCoordinator`) |

## Contributing

Contributions are welcome. Areas where help is most needed:

- Provider reference implementations (HTTP client, terminal executor, etc.)
- `IAgenticCoordinator` reference implementation
- Additional test coverage for the engine layer
- Documentation and examples

## License

MIT
