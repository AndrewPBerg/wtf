# Project tasks

Declare tasks explicitly in `.wtf/config.toml`:

```toml
version = 1
[tasks.test]
command = ["go", "test", "-p", "1", "./internal/..."]
inputs = ["go.mod", "go.sum", "internal"]
timeout_seconds = 120
```

```sh
wtf run test
wtf run test -j
wtf run test -- -run TestExample
```

The task runs in the current registered workspace. Arguments are direct argv,
not interpolated shell text; forwarded arguments preserve case. Choose `sh -c`
explicitly if shell behavior is needed. Inputs record the stage fingerprint;
tasks rerun by default rather than treating a prior success as current proof.
Omitted/zero timeout uses 120 seconds. Failures stop the task and retain records.
JSON stdout is one result; subprocess logs use stderr.

`.wtf/tasks/` is an inherited directory for project-owned supporting files.
Placing a Lua file there does not register a task. Lua callbacks can use
`wtf.exec.stage` for reusable commands with inputs/checks, cwd, and explicit child
environments. See [Lua substrate](lua-substrate.md#commands).
