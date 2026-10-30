module github.com/pulseaiclub/phi

go 1.26.3

require (
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/pulseaiclub/phi/ext/go v0.25.4
	github.com/pulseaiclub/pli v0.1.0
	github.com/pulseaiclub/sev v0.2.0
	github.com/pulseaiclub/xui v0.1.9
	github.com/stretchr/testify v1.12.1
	github.com/yuin/goldmark v1.8.6
	gopkg.in/yaml.v3 v3.0.1
)

require (
	bitbucket.org/pcastools/hash v1.0.5 // indirect
	github.com/gloathub/go-readline v0.1.2 // indirect
	github.com/gloathub/gojava v0.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	go4.org/intern v0.0.0-20220617035311-6925f38cc365 // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20230525183740-e7c30c78aeb2 // indirect
	golang.org/x/sys v0.8.0 // indirect
)

require (
	github.com/dlclark/regexp2/v2 v2.2.1 // indirect
	github.com/glojurelang/glojure v0.7.24
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)

replace github.com/pulseaiclub/phi/ext/go => ./ext/go

replace github.com/glojurelang/glojure => ../glojure
