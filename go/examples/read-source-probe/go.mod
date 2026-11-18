module github.com/aggrega-ai/aggrega-pdk/go/examples/read-source-probe

go 1.26.0

require (
	github.com/aggrega-ai/aggrega-pdk/go v0.0.0
	github.com/extism/go-pdk v1.1.3
)

require github.com/aggrega-ai/aggrega-abi/go v0.9.1 // indirect

replace github.com/aggrega-ai/aggrega-pdk/go => ../../

// The PDK's own replace directives do not apply from a consuming main
// module, so the ABI sibling is re-declared here for standalone builds.
