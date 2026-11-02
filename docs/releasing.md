# Releasing Rowan

To publish a release, run `scripts/release.sh` on `main` and push the tag that
it creates. The `Release` workflow in `.github/workflows/release.yml` then
tests the code, builds Rowan for Linux, macOS and Windows, and creates the
GitHub release.

## Before you start

You need Go at the version in `go.mod`, Node 20 or later, git and npm. On
Linux you also need `libgtk-3-dev` and `libwebkit2gtk-4.1-dev`, or the
packages with the same files on your distribution.

Add each change to the `## [Unreleased]` section of `CHANGELOG.md` when you
merge it. The release script stops when that section is empty.

## Cut a release

1. Update `main`: `git switch main && git pull --ff-only`.
2. Run the release script: `scripts/release.sh v1.2.3`.
3. Check the release commit and the tag: `git show v1.2.3`.
4. Push the commit and the tag: `git push --atomic origin main v1.2.3`.
5. Follow the workflow run: `gh run watch`.

The script does these steps in this order:

1. It checks that the tag has the form `vX.Y.Z` or `vX.Y.Z-prerelease`.
2. It checks that the branch is `main`, that the working tree is clean and
   that the tag does not exist.
3. It checks that the `## [Unreleased]` section of `CHANGELOG.md` has
   entries.
4. It runs `npm ci`, `npm test`, `npm run check` and `npm run build` in
   `frontend/`.
5. It runs `go vet` and `go test ./...`. On Linux, it adds
   `-tags webkit2_41`.
6. It moves the Unreleased entries into a new section
   `## [1.2.3] - YYYY-MM-DD`.
7. It sets `version` in `frontend/package.json` and
   `frontend/package-lock.json`, and `info.productVersion` in `wails.json`.
8. It commits `Release v1.2.3` and creates the annotated tag `v1.2.3`.

The script does not push. It prints the push command when it is done.

The Go tests in the script use a temporary config directory and file
credentials. They do not change your Rowan config or your keychain. To also
run the PostgreSQL and MySQL engine tests, start the containers from
`README.md` and give the ports:

```sh
ROWAN_TEST_PG_PORT=55432 ROWAN_TEST_MYSQL_PORT=53306 scripts/release.sh v1.2.3
```

The CI workflow always runs the engine tests.

To undo the release commit and the tag before you push, run:

```sh
git tag -d v1.2.3
git reset --hard HEAD~1
```

### Prereleases

A tag with a `-`, such as `v1.3.0-rc.1`, makes a prerelease. The workflow
marks the GitHub release as a prerelease. The script writes `1.3.0-rc.1` to
`frontend/package.json` and `1.3.0` to `wails.json`. The version fields of
Windows and macOS accept numbers only.

## What the workflow builds

The `Release` workflow starts when you push a tag that starts with `v`. It
runs these jobs:

1. `Test` calls `.github/workflows/ci.yml`. This job runs the frontend
   tests, `svelte-check`, `go vet`, `go test` against PostgreSQL 17 and
   MySQL 8.4, and a smoke build.
2. `Release notes` checks the tag format. It reads the section for the
   version from `CHANGELOG.md` and fails when the section is missing.
3. `Build` builds the 4 targets in parallel when the first 2 jobs pass.
4. `GitHub release` writes `SHA256SUMS` and runs `gh release create` with
   the notes and all files.

| Target | Runner | Wails flags |
|---|---|---|
| `linux-amd64` | `ubuntu-24.04` | `-tags webkit2_41` |
| `linux-arm64` | `ubuntu-24.04-arm` | `-tags webkit2_41` |
| `macos-universal` | `macos-latest` | `-platform darwin/universal` |
| `windows-amd64` | `windows-latest` | `-nsis` |

Each build also uses `-trimpath -m -nosyncgomod` and
`-ldflags "-X main.version=v1.2.3"`. The workflow installs the Wails CLI at
the version in `go.mod`. Before the build packs the files, it checks that
`rowan --version` prints `rowan v1.2.3`.

The workflow sets `info.productVersion` in `wails.json` from the tag before
it builds. A tag that you create without the script still gets the correct
version in `Info.plist` and in the Windows file properties.

The build jobs do not restore caches, so a poisoned cache cannot reach a
release file. The CI jobs cache Go modules and npm packages.

The Linux builds link against glibc 2.39 from Ubuntu 24.04. They run on
distributions with glibc 2.39 or later, GTK 3 and WebKitGTK 4.1.

If the `ubuntu-24.04-arm` runner is not available for your repository,
remove the `linux-arm64` entry from the matrix in
`.github/workflows/release.yml`.

### Run a release again

To repeat a failed build, click **Re-run failed jobs** on the workflow run.
If the GitHub release exists already, delete it first. This command keeps
the tag:

```sh
gh release delete v1.2.3 --yes
```

If the code needs a fix, do not move the tag. Fix the code on `main` and
release the next patch version.

## Artifact names

For the tag `v1.2.3`, the release has these files:

| File | Contents |
|---|---|
| `rowan-v1.2.3-linux-amd64.tar.gz` | The directory `rowan-v1.2.3-linux-amd64/` with the `rowan` binary, `README.md`, `rowan.desktop` and the icon `rowan.png` |
| `rowan-v1.2.3-linux-arm64.tar.gz` | The same files for arm64 |
| `rowan-v1.2.3-macos-universal.zip` | `rowan.app` for Apple silicon and Intel Macs |
| `rowan-v1.2.3-windows-amd64-installer.exe` | The NSIS installer. It installs the WebView2 runtime when it is missing |
| `rowan-v1.2.3-windows-amd64.zip` | The directory `rowan-v1.2.3-windows-amd64/` with the portable `rowan.exe` and `README.md` |
| `SHA256SUMS` | The SHA-256 checksum of each file above |

## macOS signing and notarization

Signing and notarization are optional. Without the secrets below, the
workflow gives the app an ad hoc signature. To sign the app with a
Developer ID and notarize it, add these repository secrets:

| Secret | Value |
|---|---|
| `MACOS_CERTIFICATE` | The base64 text of a `.p12` export of a Developer ID Application certificate and its private key |
| `MACOS_CERTIFICATE_PASSWORD` | The password of the `.p12` file |
| `APPLE_ID` | The Apple ID that submits the app for notarization |
| `APPLE_APP_PASSWORD` | An app-specific password for that Apple ID |
| `APPLE_TEAM_ID` | The 10-character team ID of the Apple developer account |

- When `MACOS_CERTIFICATE` and `MACOS_CERTIFICATE_PASSWORD` exist, the
  workflow signs the app with the hardened runtime and a secure timestamp.
- When the 3 Apple secrets also exist, the workflow notarizes the app and
  staples the ticket to it.
- When a secret is missing, the workflow skips that step and continues.

To add the secrets from a Mac, run these commands in the repository. Each
`gh secret set` command without input asks for the value:

```sh
base64 -i DeveloperID.p12 | gh secret set MACOS_CERTIFICATE
gh secret set MACOS_CERTIFICATE_PASSWORD
gh secret set APPLE_ID
gh secret set APPLE_APP_PASSWORD
gh secret set APPLE_TEAM_ID
```

The workflow does not sign the Windows files.

## Verify a download

`SHA256SUMS` lists the SHA-256 checksum of each file in the release. Put
`SHA256SUMS` in the same directory as the files that you downloaded. Then
run the command for your system. The `--ignore-missing` flag skips the
files that you did not download.

On Linux:

```sh
sha256sum --check --ignore-missing SHA256SUMS
```

On macOS:

```sh
shasum -a 256 --check --ignore-missing SHA256SUMS
```

On Windows, run these commands in PowerShell. The last command prints
`True` when the checksum matches:

```powershell
$file = "rowan-v1.2.3-windows-amd64-installer.exe"
$line = Select-String -Path SHA256SUMS -Pattern $file -SimpleMatch
(Get-FileHash $file -Algorithm SHA256).Hash -eq $line.Line.Split(" ")[0]
```

To download a file and `SHA256SUMS` with the GitHub CLI, run:

```sh
gh release download v1.2.3 --repo OWNER/REPO --pattern SHA256SUMS --pattern 'rowan-v1.2.3-linux-amd64.tar.gz'
```

## Unsigned builds

Builds without a Developer ID signature and without notarization start
with a warning on macOS. The Windows files have no signature, so Windows
also shows a warning.

On macOS, Gatekeeper blocks the app at the first start. To open it:

1. Move `rowan.app` to `/Applications`.
2. Open `rowan.app`. macOS shows that it cannot verify the app.
3. Open **System Settings** > **Privacy & Security**.
4. Click **Open Anyway** and confirm.

You can also remove the quarantine attribute in a terminal:

```sh
xattr -dr com.apple.quarantine /Applications/rowan.app
```

On Windows, SmartScreen shows **Windows protected your PC** for the
installer and for `rowan.exe`. Click **More info**, then click
**Run anyway**.

The Linux files have no signature. Check them with `SHA256SUMS`.
