<p align="center">
  <img src="build/appicon.png" alt="The Positron app icon" width="128" height="128">
</p>

<h1 align="center">Positron</h1>

<p align="center">
  Claude Code agents that answer your WhatsApp chats, each with its own git repo of knowledge.
</p>

<p align="center">
  macOS 14+ on Apple silicon &nbsp;·&nbsp; Uses your Claude Code login &nbsp;·&nbsp; No API key &nbsp;·&nbsp; Voice notes transcribed locally
</p>

---

## What Positron does

1. **Link WhatsApp.** Scan a QR code once. Positron becomes a linked device on your own number, like WhatsApp Web.
2. **Create agents by chatting.** Click **New agent** and say what you want, like "an agent for my family group that answers about Sunday lunch, like I would". Claude reads the chat, works out the people, the topics and how you write, asks only what it can't tell, and shows you the whole agent: settings, its instructions and what it starts out knowing. Nothing exists until you click **Create agent**.
3. **Let them work.** When a message arrives in one of its chats, the agent's Claude Code run reads the recent conversation and its repo. It replies or stays quiet, and it writes what it learned back to the repo. Positron commits that, and pushes if you want.

**Activity** has two tabs and a Claude panel:

- **Board**: blocks Claude lays out for you (today's numbers, replies waiting for you, runs per agent, what agents learned, busy hours…). Each block names a read-only query; Positron's board engine runs it and refreshes the block whenever something happens, so a live board costs no tokens. Ask Claude to change the layout in plain words. Only blocks marked **Claude** (summaries) call Claude, on the schedule you pick.
- **Log**: every run: what came in, what the agent did, what it learned. Replies from agents set to "After I approve" wait there and on the board.
- **Claude panel**: saved chats you can rename, like Mail AI's. Ask about your chats and agents, create agents, or ask for a change to an agent. Claude can read Positron's data, but anything that changes it is a card you click (Create agent, Apply), and it has no way to send WhatsApp messages.

An agent's page shows **How it behaves** (its INSTRUCTIONS.md, written by Claude). **Change with Claude** opens a chat about that agent; **Edit text** lets you edit it yourself.

## Use Positron from Claude Code

Turn on **Settings → Claude Code in other projects** and click **Add to Claude Code**. Claude Code in any folder then has Positron's tools, so you can say things like "send Ana a WhatsApp saying I'm 20 minutes late", "have the family agent tell the group lunch moved to 1pm", "approve the waiting reply in Work" or "make the Family agent reply on its own".

- There is one tool for each thing you can do in the window: chats, agents, Activity (approve, edit, discard, retry), settings, the board, Positron's Claude chats and their cards, updates and speech models. Linking a phone and logging out of WhatsApp stay in the window.
- Chats can be named by name, part of a name, phone number or JID; agents by name or ID.
- `send_whatsapp_message` sends exactly what you wrote, as you or as an agent. `ask_agent` hands a request to an agent: it runs like a normal run (its knowledge, the chat's history, its voice) and its reply goes out right away, whatever its reply mode, unless you pass `review` to have it wait in Activity. The request shows on the run in Activity.
- The server listens only on `127.0.0.1:47615` (`POSITRON_MCP_PORT` changes it) and only while the setting is on. Every request needs the token in `mcp-token` in Positron's data folder; requests from web pages (a browser `Origin`, or a `Host` other than localhost) are refused. **New token** replaces it; click **Add to Claude Code** again after. **Copy command** gives you the `claude mcp add` line to run yourself; it holds the token, so keep it private.

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

Pick an empty folder or an existing repo. Positron sets it up:

```
my-agent-kb/
├── CLAUDE.md          # how the agent keeps its memory (Claude Code reads it automatically)
├── INSTRUCTIONS.md    # who the agent is; edited in Positron, refined by the agent
├── knowledge/         # what it learned, one topic per file
└── .positron/         # photos for the current run (gitignored)
```

Each run that changes files becomes a commit, so `git log` shows what the agent learned and `git revert` undoes a bad lesson. Chat logs stay in Positron's own folder and never go into a commit.

## How a run works

```
cd <repo>
claude -p --output-format json --json-schema <reply, images, voice, note> \
  --permission-mode acceptEdits --allowedTools "Read Write Edit MultiEdit Glob Grep [WebSearch WebFetch] [Bash]" \
  --append-system-prompt <Positron's protocol> < <the last 40 messages, the new ones marked>
```

Positron drops `ANTHROPIC_API_KEY` from the environment, so runs use your Claude Code subscription.

## Install

```
git clone git@github.com:GabiHert/positron.git
cd positron
make install
```

`make install` checks for Xcode command line tools, Go, Node, Wails and Claude Code, builds `Positron.app`, signs it and puts it in /Applications. ffmpeg is needed only for voice replies (`brew install ffmpeg`).

Run `bash scripts/create-signing-identity.sh` once to sign every build with the same local certificate.

### Coming from Deputy

Positron used to be called Deputy. The first time Positron opens on a Mac that had Deputy, it brings everything over by itself: it moves `~/Library/Application Support/Deputy` to `~/Library/Application Support/Positron` (WhatsApp stays linked; agents, chats, runs and media come along), renames `deputy.sqlite` and `deputy.log`, points the stored media paths at the new folder, moves the GitHub token from the keychain item "Deputy updates" to "Positron updates", and swaps Deputy's open-at-login item for Positron's. Quit Deputy first: Positron will not move the folder while Deputy runs. Agent repos stay where they are (new agents go under `~/Positron/agents`). Afterwards, drag `Deputy.app` to the Trash. If Claude Code had Deputy's MCP server as `deputy`, click **Add to Claude Code** again to replace it with `positron`.

## Develop

```
make dev        # wails dev, with hot reload
make test       # go vet, go test, frontend typecheck and tests
POSITRON_E2E=1 go test -run TestRealClaude ./internal/engine/      # one real agent run
POSITRON_E2E=1 go test -run TestRealClaude ./internal/assistant/   # Claude proposes an agent
```

A different `POSITRON_HOME` is a separate Positron: it gets its own single-instance lock and puts new agents under that home, so you can try things next to your real one.

Data lives in `~/Library/Application Support/Positron/` (`POSITRON_HOME` overrides it): `positron.sqlite` (agents, chats, messages, runs), `whatsapp.sqlite` (the linked-device session), `media/`, `models/` and `positron.log`.

## Code

| Path | What it does |
| --- | --- |
| `internal/whatsapp` | The WhatsApp link, through [whatsmeow](https://github.com/tulir/whatsmeow) |
| `internal/engine` | Routes messages to agents, waits for bursts, runs Claude Code, delivers or holds replies, commits |
| `internal/claude` | Finds and runs your `claude` command |
| `internal/assistant` | Claude chats: runs `claude` with only Positron's tools, served over MCP on localhost; cards |
| `internal/mcpserver` | Positron's tools for Claude Code in other projects, over MCP on localhost |
| `internal/query` | The board engine's read-only queries |
| `internal/board` | Board layout, validation and Claude block schedules |
| `internal/gitrepo` | Sets up and commits knowledge repos |
| `internal/media` | Saves media, transcribes voice notes, makes voice replies |
| `internal/speech` | Local Whisper through sherpa-onnx (from Notted) |
| `internal/backend`, `internal/app` | What the window calls |
| `frontend/` | React + TypeScript UI |

## A word of caution

Automating a personal WhatsApp account is against WhatsApp's terms, and an account that sends a lot of automated messages can be banned. Keep agents on "After I approve" until you trust them, and keep the hourly limit low.

Positron borrows code from [Notted](https://github.com/GabiHert/notted), which borrows from quant. See [NOTICE.md](NOTICE.md).
