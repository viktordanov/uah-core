# uah-core-runner

Run an AI agent from a prompt or JSON request. It writes events to stdout as
JSONL and exits when the task finishes.

Install with Go 1.27+:

```sh
go install github.com/viktordanov/uah-core/cmd/uah-core-runner@latest
```

Set an OpenAI API key and run a prompt in the current directory:

```sh
export OPENAI_API_KEY="..."
uah-core-runner -p 'Inspect this project and explain how to run its tests.'
```

Or run from source at the repository root:

```sh
go run ./cmd/uah-core-runner -p 'Inspect this project and explain how to run its tests.'
```

Choose a workspace and save the output:

```sh
uah-core-runner -workspace ./my-project -p 'Summarize this project.' > run.jsonl
```

Sessions: `${XDG_STATE_HOME:-$HOME/.local/state}/uah-core/sessions`
(override with `-session-directory`).

You can also pass a JSON request as an argument or through stdin:

```sh
uah-core-runner '{"prompt":"Summarize this project."}'
uah-core-runner < request.json
```

OpenAI is the default provider. Set `UNREAL_HARNESS_LLM_PROVIDER` to `openai`,
`openai-codex`, `openrouter`, `fireworks`, or `ollama`, and
`UNREAL_HARNESS_LLM_MODEL` to choose a model. The `UNREAL_HARNESS_LLM_*`
variables keep the names of the runner uah-core derives from, so existing
setups and wrappers such as uagent keep working.

Run `uah-core-runner -h` for options and the JSON request fields.

## Docker

The [Dockerfile](../../Dockerfile) builds a Linux image of the runner. Build it
and run it with a project mounted as the workspace:

```sh
docker build -t uah-core .
docker run --rm -i --user "$(id -u):$(id -g)" \
  -e OPENAI_API_KEY -v "$PWD:/workspace" \
  -v uah-core-state:/state \
  uah-core -p 'Summarize this project.'
```
