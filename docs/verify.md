# Checking a download

Every file on the [releases page](https://github.com/confuserdotcom/tock/releases)
is built by GitHub Actions from the tagged source, never on a laptop. Two
checks, from weakest to strongest. Run them in the folder you downloaded to.

## 1. The file is the one that was published

```sh
shasum -a 256 -c SHA256SUMS --ignore-missing
```

For the `.dmg`, use its own list: `shasum -a 256 -c tock-<version>.dmg.sha256`.

`OK` next to your file means it matches the list published with the
release. This catches a broken or swapped download, not a swapped list.

## 2. GitHub built it, from this repository, at that tag

With the [GitHub CLI](https://cli.github.com):

```sh
gh attestation verify tock-<version>-linux-amd64.tar.gz -R confuserdotcom/tock
```

It checks a signed record (Sigstore) that the release workflow made the
file from a commit in confuserdotcom/tock; the output names the workflow
and the tag. It works for the tarballs, the `.dmg`, `SHA256SUMS`, the SBOM
and `tock.rb`, from v5.5.2 on (the SBOM and `SHA256SUMS` from the release after v5.5.1). Verify `SHA256SUMS` this way and step 1
becomes as strong as step 2 for every file it lists.

## What is inside

`tock-<version>.cdx.json` is the SBOM: every Go module the build uses, with
its version and hash, in CycloneDX. Feed it to a scanner
(`grype sbom:tock-<version>.cdx.json`, `osv-scanner --sbom=…`) to check for
known vulnerabilities after the release. Dependencies are also checked
with `govulncheck` before every release.

## macOS

The app is signed ad hoc, not notarized: tock has no Apple Developer ID.
macOS will say it cannot check the app for malicious software. The checks
above are what stands in for notarization: run them on the `.dmg`, then
System Settings → Privacy & Security → Open Anyway. `tock setup` then signs
what it installs with a certificate it makes on your Mac, so the
Accessibility grant survives upgrades.

Homebrew builds from the source tarball, whose sha256 is in `tock.rb`.

## Building it yourself

The surest check is not to download a binary at all:

```sh
git clone https://github.com/confuserdotcom/tock && cd tock
git checkout <version>   # a release tag, e.g. the newest on the releases page
make install          # macOS; Linux: make install-linux
```
