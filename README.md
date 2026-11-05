<p align="center">
  <img src="build/appicon.png" alt="The Deputy app icon" width="128" height="128">
</p>

<h1 align="center">Deputy</h1>

<p align="center">
  Claude Code agents that answer your WhatsApp chats, each with its own git repo of knowledge.
</p>

<p align="center">
  macOS 14+ on Apple silicon &nbsp;·&nbsp; Uses your Claude Code login &nbsp;·&nbsp; No API key &nbsp;·&nbsp; Voice notes transcribed locally
</p>

---

## What Deputy does

1. **Link WhatsApp.** Scan a QR code once. Deputy becomes a linked device on your own number, like WhatsApp Web.
2. **Create agents by chatting.** Click **New agent** and say what you want, like "an agent for my family group that answers about Sunday lunch, like I would". Claude reads the chat, works out the people, the topics and how you write, asks only what it can't tell, and shows you the whole agent: settings, its instructions and what it starts out knowing. Nothing exists until you click **Create agent**.
3. **Let them work.** When a message arrives in one of its chats, the agent's Claude Code run reads the recent conversation and its repo. It replies or stays quiet, and it writes what it learned back to the repo. Deputy commits that, and pushes if you want.

**Activity** has two tabs and a Claude panel:

- **Board**: blocks Claude lays out for you (today's numbers, replies waiting for you, runs per agent, what agents learned, busy hours…). Each block names a read-only query; Deputy's board engine runs it and refreshes the block whenever something happens, so a live board costs no tokens. Ask Claude to change the layout in plain words. Only blocks marked **Claude** (summaries) call Claude, on the schedule you pick.
- **Log**: every run: what came in, what the agent did, what it learned. Replies from agents set to "After I approve" wait there and on the board.
- **Claude panel**: saved chats you can rename, like Mail AI's. Ask about your chats and agents, create agents, or ask for a change to an agent. Claude can read Deputy's data, but anything that changes it is a card you click (Create agent, Apply), and it has no way to send WhatsApp messages.

An agent's page shows **How it behaves** (its INSTRUCTIONS.md, written by Claude). **Change with Claude** opens a chat about that agent; **Edit text** lets you edit it yourself.

## Agents

| Setting | Choices |
| --- | --- |
| Replies | On its own · After I approve · Never, only learns |
| Reacts to | Every message · Mentions and replies to you (every message in a direct chat) |
| Wait for more messages | Seconds to wait so a burst of messages gets one answer (default 8) |
| At most | Replies an hour before the rest are held for approval (default 20) |
| Prefix | Put in front of each reply (default 🤖) |
| Photos | The agent sees photos people send |
| Voice notes | Transcribed on this Mac with Whisper (download the model in Settings) |
| Send images | The agent can attach images from its repo |
| Voice replies | Answer with a spoken voice note (macOS voice + ffmpeg) |
| Tools | Read and write its repo (always) · Web search and fetch · Shell commands (off by default) |
| Model | Your Claude Code default, Sonnet, Opus or Haiku |

## The knowledge repo

Pick an empty folder or an existing repo. Deputy sets it up:

```
my-agent-kb/
├── CLAUDE.md          # how the agent keeps its memory (Claude Code reads it automatically)
├── INSTRUCTIONS.md    # who the agent is; edited in Deputy, refined by the agent
├── knowledge/         # what it learned, one topic per file
└── .deputy/           # photos for the current run (gitignored)
```

Each run that changes files becomes a commit, so `git log` shows what the agent learned and `git revert` undoes a bad lesson. Chat logs stay in Deputy's own folder and never go into a commit.

## How a run works

```
cd <repo>
claude -p --output-format json --json-schema <reply, images, voice, note> \
  --permission-mode acceptEdits --allowedTools "Read Write Edit MultiEdit Glob Grep [WebSearch WebFetch] [Bash]" \
  --append-system-prompt <Deputy's protocol> < <the last 40 messages, the new ones marked>
```

Deputy drops `ANTHROPIC_API_KEY` from the environment, so runs use your Claude Code subscription.

## Install

```
git clone git@github.com:GabiHert/deputy.git
cd deputy
make install
```

`make install` checks for Xcode command line tools, Go, Node, Wails and Claude Code, builds `Deputy.app`, signs it and puts it in /Applications. ffmpeg is needed only for voice replies (`brew install ffmpeg`).

Run `bash scripts/create-signing-identity.sh` once to sign every build with the same local certificate.

## Develop

```
make dev        # wails dev, with hot reload
make test       # go vet, go test, frontend typecheck and tests
DEPUTY_E2E=1 go test -run TestRealClaude ./internal/engine/      # one real agent run
DEPUTY_E2E=1 go test -run TestRealClaude ./internal/assistant/   # Claude proposes an agent
```

A different `DEPUTY_HOME` is a separate Deputy: it gets its own single-instance lock and puts new agents under that home, so you can try things next to your real one.

Data lives in `~/Library/Application Support/Deputy/` (`DEPUTY_HOME` overrides it): `deputy.sqlite` (agents, chats, messages, runs), `whatsapp.sqlite` (the linked-device session), `media/`, `models/` and `deputy.log`.

## Code

| Path | What it does |
| --- | --- |
| `internal/whatsapp` | The WhatsApp link, through [whatsmeow](https://github.com/tulir/whatsmeow) |
| `internal/engine` | Routes messages to agents, waits for bursts, runs Claude Code, delivers or holds replies, commits |
| `internal/claude` | Finds and runs your `claude` command |
| `internal/assistant` | Claude chats: runs `claude` with only Deputy's tools, served over MCP on localhost; cards |
| `internal/query` | The board engine's read-only queries |
| `internal/board` | Board layout, validation and Claude block schedules |
| `internal/gitrepo` | Sets up and commits knowledge repos |
| `internal/media` | Saves media, transcribes voice notes, makes voice replies |
| `internal/speech` | Local Whisper through sherpa-onnx (from Notted) |
| `internal/backend`, `internal/app` | What the window calls |
| `frontend/` | React + TypeScript UI |

## A word of caution

Automating a personal WhatsApp account is against WhatsApp's terms, and an account that sends a lot of automated messages can be banned. Keep agents on "After I approve" until you trust them, and keep the hourly limit low.

Deputy borrows code from [Notted](https://github.com/GabiHert/notted), which borrows from quant. See [NOTICE.md](NOTICE.md).
