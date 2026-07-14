# GitHub Pages Documentation Site

The documentation portal is a static GitHub Pages site built from Markdown files in `docs/` plus release artifacts from the repository root.

Canonical URL:

```text
https://alekpopovic.github.io/orch/
```

Repository README and cross-document documentation links point at this GitHub Pages portal so readers land on the styled documentation experience instead of raw Markdown.

## What Gets Published

The Pages workflow publishes:

- `docs/index.html` as the documentation shell.
- `docs/assets/` for the Tailwind-based UI behavior and styling.
- All Markdown files from `docs/`.
- `README.md`, `CHANGELOG.md`, and `RELEASE_NOTES.md` for release-oriented pages.
- `api/openapi.yaml` as `openapi.yaml` for the API contract page.
- SVG brand assets and Mermaid chart pages for a colorful release portal.

## Theme Modes

The site supports three theme modes:

- `Auto` follows the operating system preference.
- `Light` forces the light palette.
- `Dark` forces the dark palette.

The selected mode is stored in local browser storage.

## Charts

The site loads Mermaid and renders fenced `mermaid` code blocks from Markdown. README and `CHARTS.md` use this for architecture, release-focus, and lifecycle diagrams.

## Deployment

GitHub Actions deploys the site through `.github/workflows/pages.yml` on pushes to `main` that touch documentation, release notes, the OpenAPI spec, or the Pages workflow.

Manual deployment is available from the workflow dispatch button in GitHub Actions.

## Local Preview

To preview the docs shell locally from the repository root:

```sh
rm -rf /tmp/orch-docs-site
mkdir -p /tmp/orch-docs-site
cp -R docs/. /tmp/orch-docs-site/
cp README.md CHANGELOG.md RELEASE_NOTES.md /tmp/orch-docs-site/
cp api/openapi.yaml /tmp/orch-docs-site/openapi.yaml
python3 -m http.server 8088 --directory /tmp/orch-docs-site
```

Then open `http://localhost:8088`.
