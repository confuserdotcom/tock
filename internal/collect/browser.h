#ifndef TOCK_BROWSER_H
#define TOCK_BROWSER_H

// Script kinds. The numbers are shared with browserKind in browser_darwin.go.
#define TOCK_SCRIPT_CHROME 1
#define TOCK_SCRIPT_ARC 2
#define TOCK_SCRIPT_BRAVE 3
#define TOCK_SCRIPT_SAFARI 4
#define TOCK_SCRIPT_SELFTEST_URL 90
#define TOCK_SCRIPT_SELFTEST_ERROR 91

// tock_run_script runs the constant script for kind (the 3.0 s warm-up
// variant when warm is non-zero) on the main thread. It returns 0 and copies
// the string result into out on success, or returns 1 and stores the
// AppleScript error number in *errnum.
int tock_run_script(int kind, int warm, char *out, int outlen, int *errnum);

// tock_automation_check asks TCC for the Automation status of bundle on a
// background queue (never the main thread) and reports it through the Go
// export goAutomationResult. ask non-zero lets macOS show the consent prompt.
void tock_automation_check(const char *bundle, int ask);

#endif
