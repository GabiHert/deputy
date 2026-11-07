# Handoff: install Deputy on the work Mac

## Who you are helping

The person builds their own dev tools on a personal Mac. This session runs on their managed work
Mac. Your job is to install **Deputy** here from a pre-built GitHub Release, check that it can
run, and report back. From then on Deputy updates itself.

Deputy is a macOS app (Go + Wails) that runs Claude Code agents on the person's WhatsApp chats.
Repo: `GabiHert/deputy` (private). Its personal-Mac session wrote this handoff on 2026-11-06.

**Prerequisite:** a published release at `https://github.com/GabiHert/deputy/releases/latest`.
When this was written, none had been published yet. If there is still none, stop after step 0 and
say so.

## How Deputy updates (already built)

- `make release` on the personal Mac builds Deputy, tags it and attaches two files to a GitHub
  Release:
  - `Deputy-vX.Y.Z-darwin-arm64.zip`
  - `SHA256SUMS`
- The app calls `https://api.github.com/repos/GabiHert/deputy/releases/latest`. It downloads the
  zip through the API asset URL and checks it against `SHA256SUMS` and its code signature. Then it
  swaps the app bundle and rolls back if the new one does not start.
- Go's HTTP client trusts the macOS system keychain, so the security agent's CA should be accepted.
- A GitHub token for the private repo lives in the macOS login keychain, under the service
  "Deputy updates". The person adds it in Deputy → Settings → Updates.
- **Signing:** releases are signed ad hoc and not notarized until the person signs `make release`
  with an Apple Developer ID. Once a Developer ID build is installed, the updater refuses updates signed
  differently.

## Hard boundaries

Do not do any of these, even if something fails:

- Clone, fetch or pull any repo, or try git over SSH.
- Use tunnels or proxies, or mirror code or files through other hosts.
- Disable, reconfigure or work around the endpoint security agent or its TLS inspection.
- Turn off certificate checks (`curl -k`, `NODE_TLS_REJECT_UNAUTHORIZED=0` and so on).
- Install or build anything with npm, pip, brew, go or make.
- Get around Gatekeeper. That means no `xattr -d com.apple.quarantine`, no `spctl --master-disable`
  or `--add`, and no `codesign` re-signing. Do not download with curl just to skip the quarantine
  flag, either.
- Change MDM profiles or system security settings.

If a step is blocked, stop, report exactly what was blocked, and let the person decide.
**Open Anyway** in System Settings → Privacy & Security is fine **only if** macOS offers it and the
person chooses it. That is the normal user path, allowed by their policy.

## Step 0: preflight (read-only)

Run these and record the output:

```sh
sw_vers; uname -m                                    # want arm64
spctl --status                                       # Gatekeeper state (read only)
profiles status -type enrollment 2>/dev/null         # MDM enrollment (read only)
curl -sS -o /dev/null -w '%{http_code}\n' https://api.github.com                         # want 200
curl -sS -o /dev/null -w '%{http_code}\n' https://api.github.com/repos/GabiHert/deputy/releases/latest
#   404 without a token is expected for a private repo
curl -sS -o /dev/null -w '%{http_code}\n' https://web.whatsapp.com                       # WhatsApp link
command -v claude && claude --version                # Deputy runs the claude CLI
command -v git; command -v ffmpeg; command -v gh     # optional, see below
ls -ld /Applications ~/Applications 2>/dev/null; test -w /Applications && echo apps-writable
```

What the tools do in Deputy:

- `claude`: required. Agents run `claude -p`. It must be installed and logged in, the way the
  company allows. If it isn't, stop and report.
- `git`: agents keep their knowledge in local git repos. Local git is fine; pushing to GitHub is not
  expected to work here. Keep each agent's **Push** setting off.
- `ffmpeg`: only for voice replies and some media. Without it those features fail, and the rest
  works. Do not install it.
- `gh`: optional fallback for the token. It isn't needed.

## Step 1: a GitHub token (the person does this)

The person creates a fine-grained token in a browser:

- Settings → Developer settings → Fine-grained tokens.
- Resource owner: GabiHert. Repository: only `GabiHert/deputy`.
- Permissions: Contents **read-only**, and nothing else.

Do not ask them to paste the token into this chat. They paste it into Deputy in step 4. If you
need it for a check in step 2, read it into a shell variable without echoing it, and unset it
afterwards.

## Step 2: download the first build

Prefer the browser. Downloads made in the browser carry the quarantine flag, so Gatekeeper judges
the app normally.

- If the browser is signed in to GitHub with access to the repo, open
  `https://github.com/GabiHert/deputy/releases/latest` and download
  `Deputy-<tag>-darwin-arm64.zip` and `SHA256SUMS`.
- If not, ask the person how they want to get it. Don't pick a route that skips quarantine.
  If they choose the API with their token, mark the zip as downloaded afterwards, so Gatekeeper
  still assesses it:

  ```sh
  # list the assets (read the token without echoing it)
  read -rs GH_TOKEN; export GH_TOKEN
  curl -sS -H "Authorization: Bearer $GH_TOKEN" https://api.github.com/repos/GabiHert/deputy/releases/latest \
    | python3 -c 'import json,sys; [print(a["id"], a["name"]) for a in json.load(sys.stdin)["assets"]]'
  # download one asset by id (-L follows the redirect; curl drops the token on the new host)
  curl -sSL -H "Authorization: Bearer $GH_TOKEN" -H "Accept: application/octet-stream" \
    -o ~/Downloads/<name> https://api.github.com/repos/GabiHert/deputy/releases/assets/<id>
  unset GH_TOKEN
  # keep Gatekeeper in the loop, as a browser download would
  xattr -w com.apple.quarantine "0081;$(printf %x "$(date +%s)");curl;" ~/Downloads/Deputy-*.zip
  ```

Verify the checksum before unzipping:

```sh
cd ~/Downloads && shasum -a 256 -c SHA256SUMS --ignore-missing
```

## Step 3: install and open

```sh
# /Applications if writable, else ~/Applications. Deputy must be able to write to this folder to update itself.
mkdir -p ~/Applications
ditto -x -k ~/Downloads/Deputy-*-darwin-arm64.zip ~/Applications/
codesign -dvv ~/Applications/Deputy.app 2>&1 | grep -E 'Identifier|TeamIdentifier|Signature'
spctl --assess --type execute -vv ~/Applications/Deputy.app   # read only: what Gatekeeper thinks
open ~/Applications/Deputy.app
```

- If it opens, go to step 4.
- If macOS blocks it, record the exact dialog text and the `spctl` output. Tell the person whether
  Privacy & Security shows **Open Anyway**; using it is their call. If there is no such option, stop.
  The fix is on the personal-Mac side: a Developer ID-signed, notarized release, or approval from IT.

## Step 4: first run

1. Link WhatsApp: Deputy shows a QR code (or a phone-number code), and the person scans it on the
   phone.
2. **Agents:** this Deputy starts with no agents. If the personal Mac's Deputy is also linked and
   running agents, do not recreate the same agents here, or both Macs will answer the same chats.
3. Settings → Updates → **Add token**: the person pastes the token, then clicks **Save in
   keychain**.
4. Click **Check for updates**. Expect "Deputy is up to date", or a newer version.

## Step 5: report back

Send this filled in to the person, for their personal-Mac session:

```
Mac: <sw_vers / arch>     Gatekeeper: <spctl --status>     MDM: <enrolled?>
api.github.com: <code>    web.whatsapp.com: <code>
claude: <version or missing>   git: <yes/no>   ffmpeg: <yes/no>
Release downloaded via: <browser / API + quarantine>   checksum: <OK / FAIL>
Signature: <Identifier, TeamIdentifier, adhoc?>   spctl --assess: <output>
Opened: <yes / blocked: exact text / Open Anyway used>
WhatsApp linked: <yes / no: error>
Check for updates: <message shown>
Anything blocked: <what, exact error>
```
