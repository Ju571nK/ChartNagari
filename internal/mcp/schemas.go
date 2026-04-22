package mcp

// JSON Schema constants for tool input validation. Served verbatim to MCP
// clients via tools/list. Keep schemas minimal — LLM gets what it needs from
// the tool description string, not schema detail.

const SchemaListWatchlist = `{
  "type": "object",
  "properties": {},
  "additionalProperties": false
}`

const SchemaGetAnalysis = `{
  "type": "object",
  "properties": {
    "symbol": {"type": "string", "description": "Symbol from watchlist, e.g. BTCUSDT or AAPL"}
  },
  "required": ["symbol"],
  "additionalProperties": false
}`

const SchemaGetSignalHistory = `{
  "type": "object",
  "properties": {
    "symbol": {"type": "string"},
    "since":  {"type": "string", "description": "ISO 8601 timestamp. Default: 7 days ago."},
    "limit":  {"type": "integer", "minimum": 1, "maximum": 200, "default": 50}
  },
  "required": ["symbol"],
  "additionalProperties": false
}`

const SchemaGetOHLCV = `{
  "type": "object",
  "properties": {
    "symbol":    {"type": "string"},
    "timeframe": {"type": "string", "enum": ["1W","1D","4H","1H"]},
    "limit":     {"type": "integer", "minimum": 1, "maximum": 500, "default": 50}
  },
  "required": ["symbol","timeframe"],
  "additionalProperties": false
}`

const SchemaGetEconomicCalendar = `{
  "type": "object",
  "properties": {
    "start":      {"type": "string", "description": "ISO 8601 timestamp"},
    "end":        {"type": "string", "description": "ISO 8601 timestamp"},
    "impact_min": {"type": "string", "enum": ["low","medium","high"], "default": "medium"}
  },
  "required": ["start","end"],
  "additionalProperties": false
}`
