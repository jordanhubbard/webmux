package recovery

// CodexFilesHelper is a private metadata worker dispatched before CLI aliases.
// Windows file-name queries can block inside the kernel; isolating them lets
// the parent enforce a hard deadline without leaking a blocked goroutine.
const CodexFilesHelper = "--recovery-codex-files"
