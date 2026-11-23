// This nested module keeps frontend dependencies out of the root Go module's
// `./...` package scan. The web directory is owned by npm tooling, not Go.
module github.com/NSObjects/PersonaOS/web

go 1.26
