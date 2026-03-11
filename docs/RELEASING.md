# Releasing Gitshelf

## How it works

When you push a git tag (`v*`), GitHub Actions runs tests then GoReleaser, which:

1. Cross-compiles for macOS, Linux, Windows (amd64 + arm64)
2. Creates a GitHub Release with tarballs/zips
3. Pushes a Homebrew formula to `ignaciotcrespo/homebrew-tap`

## First-time setup

### 1. Create the Homebrew tap repo

Create a new empty repo on GitHub: `ignaciotcrespo/homebrew-tap`

### 2. Create a personal access token

- GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens
- Name: `HOMEBREW_TAP_GITHUB_TOKEN`
- Repository access: select `homebrew-tap` only
- Permissions: Contents → Read and write

### 3. Add the token as a repo secret

- Go to `ignaciotcrespo/gitshelf` → Settings → Secrets and variables → Actions
- New repository secret: `HOMEBREW_TAP_GITHUB_TOKEN` = the token from step 2

### 4. Add a LICENSE file

Homebrew formulas expect a license. Create a `LICENSE` file in the repo root.

## Publishing a release

```bash
git tag v0.1.0
git push origin main --tags
```

The workflow runs automatically. Check progress at Actions tab on GitHub.

## Publishing a pre-release / snapshot

Use a pre-release suffix. GoReleaser marks it as pre-release on GitHub and skips the Homebrew tap automatically (`skip_upload: auto` in `.goreleaser.yml`).

```bash
git tag v0.1.0-alpha.1
git push origin v0.1.0-alpha.1
```

## Deleting a release

You can delete any release from GitHub. This removes the release page and its downloadable assets, but users who already installed it are unaffected.

```bash
gh release delete v0.1.0 --yes        # delete the GitHub release
git push origin --delete v0.1.0       # delete the remote tag
git tag -d v0.1.0                     # delete the local tag
```

> **Note:** If the release was published to Homebrew, the formula in `homebrew-tap` will still point to the deleted version. Push a new release to update it, or manually remove the formula from the tap repo.

## How users install

```bash
brew tap ignaciotcrespo/tap
brew install gitshelf
```

Or without tapping first:

```bash
brew install ignaciotcrespo/tap/gitshelf
```

## Local build with version

```bash
./build.sh
./gitshelf --version
```

## Testing the release config locally

```bash
goreleaser check                    # validate .goreleaser.yml
goreleaser release --snapshot --clean  # dry-run build without publishing
```

