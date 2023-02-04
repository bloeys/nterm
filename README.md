# nTerm

## Developer Notes

To build without a CLI showing on Windows build with: `go build -ldflags -H=windowsgui .`.

### OS Manifests

To ensure we get proper configuration on each OS, we sometimes need extra files that are part of the compilation.
This ensures we do get things like icons and proper DPI awareness (which is important for crisp text).

Those manifests are turned into files (e.g. `*.syso`) that get detected and used by `go build` automatically.

#### Windows

The Windows manifest and icon are placed inside the `.winres` folder, which gets compiled into the required
`*.syso` files by running `go-winres make --in ./.winres/winres.json`.

`go-winres` can be installed using `go install github.com/tc-hib/go-winres@latest`.

**Note:** Any changes to things in `.winres` requires re-running the `go-winres` command and then recompiling the Go program.
