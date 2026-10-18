# aileena-code

A code-editing agent in **one Go file, under 400 lines, zero dependencies**.

It is a faithful, step-by-step build of Thorsten Ball's
[**How to Build an Agent**](https://ampcode.com/how-to-build-an-agent)
("or: The Emperor Has No Clothes"). All credit for the idea, the structure and
the tool designs goes to that article. Read it first, then read `main.go`.

> It's an LLM, a loop, and enough tokens. — Thorsten Ball

Public and free, MIT licensed.

## Why this exists

I wanted to read the real code of an agent harness with nothing in the way.
This repo is that code: a chat loop, three tools (`read_file`, `list_files`,
`edit_file`), and the inner loop that runs the tools the model asks for.

## The git history is the tutorial

Every article section is exactly one commit. Run `git log --reverse -p` to
replay the tutorial.

| Step | Commit | Article section | What changes |
| --- | --- | --- | --- |
| 1 | `Step 1: chat loop (the heartbeat)` | *Pencils out!* | `Agent`, `Run`, `runInference`, the client |
| 2 | `Step 2: tool plumbing` | *A First Tool* | `ToolDefinition`, tools sent with each request, `executeTool`, the inner loop |
| 3 | `Step 3: read_file tool` | *The `read_file` tool* | first tool and the schema helper |
| 4 | `Step 4: list_files tool` | *The `list_files` tool* | directory walk |
| 5 | `Step 5: edit_file tool` | *Let it `edit_file`* | string-replace edits, file creation |

## Where everything is in `main.go`

| Lines | What | Article equivalent |
| --- | --- | --- |
| 19–36 | `main`: reads stdin, registers the three tools | `main()` |
| 38–50 | `NewAgent`, `Agent` | same |
| 52–90 | `Run`: the heartbeat. Read input, call the model, run tools, loop. `readUserInput` (line 57) is false while tool results are being sent back | `Run()` (second version) |
| 92–107 | `executeTool`: look up the tool by name, run it, wrap the result | `executeTool()` |
| 109–121 | `runInference`: send the conversation and tool definitions | `runInference()` |
| 125–143 | `ToolDefinition`, `schema`, `prop` | `ToolDefinition`, `GenerateSchema` |
| 145–171 | `read_file` | same |
| 173–227 | `list_files` | same |
| 229–293 | `edit_file`, `createNewFile` | same |
| 295–396 | `Client`: OpenAI-compatible `/chat/completions` over `net/http` | `anthropic.NewClient()` and the SDK types |

## How it differs from the article

- **Provider.** The article uses Anthropic's Go SDK. This version speaks the
  OpenAI-compatible chat-completions format with plain `net/http`, so it works
  with DeepSeek (the default), OpenAI, OpenRouter, vLLM, Ollama and others.
  The mapping: `tool_use` blocks become `tool_calls`, `tool_result` blocks
  become `role: "tool"` messages with a `tool_call_id`, and `input_schema`
  becomes `function.parameters`. To use Claude, point `AGENT_BASE_URL` at an
  OpenAI-compatible endpoint, or swap `Client.Chat` for the SDK exactly as the
  article shows; nothing else changes.
- **DeepSeek thinking mode.** `deepseek-flash` thinks by default. When tools
  are present DeepSeek requires the previous `reasoning_content` to be sent
  back, so `Message` keeps that field and the loop echoes it.
- **Schemas** are written by hand (`schema`, `prop`) instead of generated with
  `invopop/jsonschema`, to keep zero dependencies.
- **Small fixes:** bad tool input returns an error to the model instead of
  `panic`; `list_files` skips `.git`; a missing tool returns an error result.

## Sharp edges (kept on purpose, same as the article)

- No sandbox. `read_file` and `edit_file` can touch any path the process can.
  Run it in a scratch directory or a throwaway checkout.
- `edit_file` replaces **every** match of `old_str`, even though its
  description asks the model for a unique match.
- No context management, no retries, no step cap. Long sessions will
  eventually overflow the context window.

These are exactly the places where a real harness starts.

## How it relates to my other agents

- **`hx` (harness-cli)** is a harness *around* agents: it runs them, records
  what they do and tests them. `aileena-code` is the smallest possible thing
  for `hx` to run: the same loop with no extras.
- **Machina**, the agent on [aileena.xyz](https://www.aileena.xyz), runs the
  same loop (model, tool calls, results, model again) inside a Next.js API
  route. It adds what this file leaves out: a 4-step cap on the loop
  (`reactGuard`), a model router with a circuit breaker, a frozen prompt prefix
  for caching, owner-only modes, quotas, and action IDs so retried side
  effects never run twice. If you want to see which parts of Machina are
  the agent and which parts are elbow grease, compare it with this file.
  Machina's code mode ([`lib/codeAgent/agent.ts`](https://github.com/lilaclilac09/aileen_machina_01/blob/main/aileena-new/lib/codeAgent/agent.ts))
  is this loop ported to TypeScript, with the same names.

## Run it

Needs Go 1.24+.

```shell
git clone https://github.com/lilaclilac09/aileena-code
cd aileena-code
export DEEPSEEK_API_KEY=...        # never commit this
go run .
```

| Variable | Default | Purpose |
| --- | --- | --- |
| `DEEPSEEK_API_KEY` | | API key (used when `AGENT_API_KEY` is unset) |
| `AGENT_API_KEY` | `$DEEPSEEK_API_KEY` | key for any other provider |
| `AGENT_BASE_URL` | `https://api.deepseek.com` | any OpenAI-compatible base URL |
| `AGENT_MODEL` | `deepseek-flash` | model id |

Try the article's prompts:

```shell
echo 'what animal is the most disagreeable because it always says neigh?' >> secret-file.txt
go run .
# You: help me solve the riddle in the secret-file.txt file
# You: create fizzbuzz.js that I can run with Node.js and that has fizzbuzz in it and executes it
```

## Tests

```shell
go vet ./... && go test ./...
```

`main_test.go` runs the whole loop offline against a mock OpenAI-compatible
server. The mock asks for `read_file`, and the test checks that the tool
result and `reasoning_content` are sent back before the final answer. It
also covers `edit_file` (create, replace, errors) and `list_files`.

## Demo

The Go binary has not been run against a live model yet: there was no API key
in the build environment, so it was checked with `go build`, `go vet` and the
offline tests only. A real transcript will be added after the first run with
a key.

The same loop, ported to TypeScript, runs live in Machina. Anyone can call it
through the public MCP tool `suggest_patch` at `https://www.aileena.xyz/api/mcp`
(see [docs/mcp-github.md](https://github.com/lilaclilac09/aileen_machina_01/blob/main/aileena-new/docs/mcp-github.md)).
Real output from 18 October 2026, for the article's FizzBuzz task (a `.env`
file was sent too and was refused):

```diff
--- a/fizzbuzz.js
+++ b/fizzbuzz.js
@@ -1,7 +1,15 @@
 function fizzBuzz(n) {
   for (let i = 1; i <= n; i++) {
-    console.log(i);
-  }
-}
-
-fizzBuzz(100);
+    if (i % 15 === 0) {
+      console.log("FizzBuzz");
+    } else if (i % 3 === 0) {
+      console.log("Fizz");
+    } else if (i % 5 === 0) {
+      console.log("Buzz");
+    } else {
+      console.log(i);
+    }
+  }
+}
+
+fizzBuzz(15);
```

```text
$ git apply p.diff && node fizzbuzz.js
1 2 Fizz 4 Buzz Fizz 7 8 Fizz Buzz 11 Fizz 13 14 FizzBuzz
```

## License

MIT. See [LICENSE](LICENSE).
