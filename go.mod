module github.com/Matir/sshdog

go 1.18

require (
	github.com/GeertJohan/go.rice v1.0.2
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/pkg/term v1.1.0
	golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a
	golang.org/x/sys v0.0.0-20210615035016-665e8c7367d1
)

require github.com/daaku/go.zipexe v1.0.0 // indirect

replace golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a => github.com/drakkan/crypto v0.0.0-20220615080207-8cff98973996
