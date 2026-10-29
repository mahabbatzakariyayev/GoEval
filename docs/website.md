# Publishing the documentation

The documentation website is generated from the Markdown guides in this
repository. It uses Go's standard library and static HTML, CSS, and JavaScript.

Read the [public GoEval documentation](https://goeval-docs.mahabbatzakariya.chatgpt.site).
It is available to everyone without signing in.

## Build the site

From the repository root:

```sh
go run ./internal/cmd/docs
```

The generated files are written to website/dist. No Node, Python, or separate
static-site generator is needed.

The builder validates local page links, anchors, navigation, and assets. It also
generates a local search index and includes LICENSE and NOTICE.

## Preview locally

```sh
go run ./internal/cmd/docs -serve 127.0.0.1:4173
```

Open [the local preview](http://127.0.0.1:4173). Re-run the command after changing
content. Stop the server with Ctrl+C.

## Publish on GitHub Pages

The repository includes a documentation workflow. It builds and checks the site
for pull requests. On pushes to main, it deploys the generated artifact using
GitHub Pages.

Enable GitHub Pages in the repository's Settings → Pages and select GitHub Actions
as the publishing source. Then push the reviewed documentation changes. The
expected project URL is:

```text
https://mahabbatzakariyayev.github.io/GoEval/
```

This is the intended GitHub Pages URL; it is available only after a successful
deployment. Check the workflow's deployment output for the actual published URL.

See [GitHub's custom workflow guide](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages)
for repository publishing-source settings and deployment permissions.

The site uses relative URLs, so it also works from a different subpath or a
static host. The public Sites deployment uses the same generated files.

## Edit a guide

Guides live in docs/guides. Navigation, page titles, output names, and search
descriptions are declared in website/pages.json. Styles and browser interactions
live in website/assets; the page shell lives in website/template.html.

The renderer supports the Markdown used by these guides: headings, paragraphs,
fenced code, lists, tables, links, inline code, bold text, and blockquotes.
It escapes raw HTML and rejects unsupported link schemes.

## Validate documentation

```sh
go run ./internal/cmd/check-docs
go test ./internal/docsite
go run ./internal/cmd/docs
```

Complete Go examples in README and the guides are compiled from separate
consumer modules. Model examples are built without making model requests.
The offline examples can also be run directly from the examples directory.

Generated website/dist files are ignored by the library repository. Build them
again for each deployment. The content source remains Markdown.
