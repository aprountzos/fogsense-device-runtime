# Maintainer notes

Instructions for whoever publishes and maintains this repository.

## 1. Push to GitHub

```bash
git init
git branch -M main
git add .
git commit -m "Initial commit"
gh repo create fogsense-device-runtime --public --source=. --remote=origin --push
# or: git remote add origin https://github.com/<user>/fogsense-device-runtime.git && git push -u origin main
```

## 2. GitHub Pages (Docusaurus site in `docs/`)

`.github/workflows/deploy-docs.yml` builds and deploys the site on every push to `main`.

1. Repository **Settings → Pages → Build and deployment → Source: GitHub Actions**.
2. In `docs/docusaurus.config.js` set `organizationName` (your user/organization), `projectName`,
   `url` (`https://<user>.github.io`) and `baseUrl` (`/fogsense-device-runtime/`). A wrong `baseUrl`
   is the usual reason for a published site without styles or with broken links.
3. Push, or run **Actions → Deploy Docs to GitHub Pages → Run workflow**.

The site is then at `https://<user>.github.io/fogsense-device-runtime/`.

## 3. Continuous integration

`.github/workflows/ci.yml` runs on every push and pull request: Go tests (Go 1.22, 1.23, 1.24 on
Ubuntu and Windows) and a Docusaurus build that must have no broken links.

## 4. Docker Hub publishing

`.github/workflows/docker-publish.yml` builds `linux/amd64` and `linux/arm64` images and pushes them
on merges to `main` and on tags `v*.*.*`.

Repository secrets (**Settings → Secrets and variables → Actions**):
`DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` (an access token with Read & Write permission).

Tag `v1.0.0` publishes `1.0.0`, `1.0` and `1`; a push to `main` also moves `latest`.

## 5. Release hygiene for a course (important)

* **Never change the behavior of a tag after announcing it.** Fix bugs in a new patch tag and
  announce it. Contract changes (new spec fields, new reason codes, different heartbeat) must be
  additive.
* Before announcing a tag, run the conformance check against every file in `examples/` (see the
  README, section *Tests*) and measure the image size and the memory use of one container
  (`docker images`, `docker stats`), so the numbers you publish are measured ones.
* Keep exam material (unseen specs, the control token, the exam procedure) **out of this public
  repository**.
