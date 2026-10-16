package claude

import "encoding/json"

const summarySystem = `You write the daily summary for tock, a personal time tracker. The user message is one day of the user's own activity as JSON: work blocks with their time, label (project and category), and the apps, sites or window titles inside them with minutes. Write what the person worked on, plainly and concretely, in the second person ("You spent the morning on…"), from the given data only: never guess at content you cannot see. Call the summarize tool once: "day" is 2 to 4 short sentences about the whole day; "blocks" has one entry per block id, each at most 15 words.`

var summaryTool = json.RawMessage(`{
  "name": "summarize",
  "description": "The day's summary and one line per work block.",
  "input_schema": {
    "type": "object",
    "properties": {
      "day": {"type": "string"},
      "blocks": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {"id": {"type": "string"}, "summary": {"type": "string"}},
          "required": ["id", "summary"]
        }
      }
    },
    "required": ["day", "blocks"]
  }
}`)

const askSystem = `You translate a question about the user's tracked time into one query for tock, a personal time tracker, which runs it on the user's machine. You never see the data. The user message gives today's date, the user's categories (with their kind: deep, shallow, distract, neutral) and projects, and the question. Choose the range the question means (default: last 7 days), what to group by, and filters. Use names exactly as listed for category and project. For a site use its host (e.g. "youtube.com"); for an app its name as the user wrote it. Call the query tool once. If the question is not about tracked time, set "unsupported" to a short reason.`

var askTool = json.RawMessage(`{
  "name": "query",
  "description": "A query over the user's tracked time.",
  "input_schema": {
    "type": "object",
    "properties": {
      "range": {"type": "string", "enum": ["today", "yesterday", "this_week", "last_week", "this_month", "last_month", "last_days", "dates"]},
      "days": {"type": "integer", "description": "for last_days"},
      "from": {"type": "string", "description": "YYYY-MM-DD, for dates"},
      "to": {"type": "string", "description": "YYYY-MM-DD inclusive, for dates"},
      "group_by": {"type": "string", "enum": ["total", "category", "project", "app", "site", "kind", "day"]},
      "category": {"type": ["string", "null"]},
      "project": {"type": ["string", "null"]},
      "app": {"type": ["string", "null"]},
      "site": {"type": ["string", "null"]},
      "kind": {"type": ["string", "null"], "enum": ["deep", "shallow", "distract", "neutral", "browse", null]},
      "limit": {"type": "integer"},
      "unsupported": {"type": ["string", "null"]}
    },
    "required": ["range", "group_by"]
  }
}`)
