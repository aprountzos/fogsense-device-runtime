---
id: versioning
title: Versions and Releases
sidebar_label: Versions & Releases
sidebar_position: 1
---

# Versions and Releases

FogSense uses Semantic Versioning (SemVer) for runtime releases and Docusaurus versioning for release-specific documentation.

## Runtime version format

A release tag follows:

```text
vMAJOR.MINOR.PATCH
```

- MAJOR: incompatible changes to device specs, APIs, gateway contracts or runtime behavior.
- MINOR: backward-compatible functionality, such as adding a new supported transport or device capability.
- PATCH: backward-compatible fixes and documentation corrections.

Until the first stable release, changes are developed on feature branches and merged through pull requests. The GitHub Releases page is the canonical place for published release notes and downloadable assets.

## Documentation versioning policy

The documentation currently describes the development version. We should create a versioned documentation snapshot when the first runtime release is published, rather than creating a new docs version for every commit.

For each release:

1. Make sure the release candidate has passed the Go, Docker and documentation CI jobs.
2. From the docs/ directory, create the matching Docusaurus docs snapshot:

   ```bash
   npm run docusaurus -- docs:version 0.1.0
   ```

3. Commit the generated versioned_docs/, versioned_sidebars/ and versions.json files.
4. Add a docsVersionDropdown item to the Docusaurus navbar so readers can switch between the current development docs and released documentation.
5. Create a Git tag such as v0.1.0 on that final release commit.
6. Create a GitHub Release for the tag with release notes and migration guidance.

The versioned documentation must be generated from the same commit/tag as the runtime release. Do not edit a released documentation snapshot to describe features that shipped later.

## Release checklist

- [ ] Go tests, vet and builds pass.
- [ ] Docker image builds successfully.
- [ ] Docusaurus production build succeeds.
- [ ] HTTP behavior remains backward compatible.
- [ ] MQTT topic and payload contract is documented.
- [ ] README, environment-variable reference and examples are current.
- [ ] CHANGELOG entry is complete.
- [ ] Git tag and GitHub Release use the same version.
- [ ] Versioned Docusaurus docs are generated from the release commit.

## Where to find releases

See [FogSense GitHub Releases](https://github.com/aprountzos/fogsense-device-runtime/releases) for published versions and release notes. The [CHANGELOG](https://github.com/aprountzos/fogsense-device-runtime/blob/main/CHANGELOG.md) tracks unreleased and historical changes.
