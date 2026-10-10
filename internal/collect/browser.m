// Browser URL capture through in-process NSAppleScript (D-06). Every script
// source below is a compile-time constant selected by an integer kind: no
// data is ever interpolated into a script (T-1-asinject). Each script is
// compiled once and cached.
#import <Cocoa/Cocoa.h>
#include <CoreServices/CoreServices.h>
#include <stdlib.h>
#include <string.h>

#include "_cgo_export.h"
#include "browser.h"

// Normal variants: 0.5 s bound so a hung browser costs at most that much
// main-thread time per poll.
static NSString *const kChromeScript =
    @"with timeout of 0.5 seconds\n"
     "tell application \"Google Chrome\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "if (mode of front window) is \"incognito\" then return \"private\"\n"
     "return URL of active tab of front window\n"
     "end tell\n"
     "end timeout";
static NSString *const kArcScript =
    @"with timeout of 0.5 seconds\n"
     "tell application \"Arc\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "if (mode of front window) is \"incognito\" then return \"private\"\n"
     "return URL of active tab of front window\n"
     "end tell\n"
     "end timeout";
static NSString *const kBraveScript =
    @"with timeout of 0.5 seconds\n"
     "tell application \"Brave Browser\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "if (mode of front window) is \"incognito\" then return \"private\"\n"
     "return URL of active tab of front window\n"
     "end tell\n"
     "end timeout";
static NSString *const kSafariScript =
    @"with timeout of 0.5 seconds\n"
     "tell application \"Safari\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "return URL of current tab of front window\n"
     "end tell\n"
     "end timeout";

// Warm-up variants: the first Apple Event to a browser in a process can take
// seconds while the target loads its scripting dictionary.
static NSString *const kChromeWarmScript =
    @"with timeout of 3.0 seconds\n"
     "tell application \"Google Chrome\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "if (mode of front window) is \"incognito\" then return \"private\"\n"
     "return URL of active tab of front window\n"
     "end tell\n"
     "end timeout";
static NSString *const kArcWarmScript =
    @"with timeout of 3.0 seconds\n"
     "tell application \"Arc\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "if (mode of front window) is \"incognito\" then return \"private\"\n"
     "return URL of active tab of front window\n"
     "end tell\n"
     "end timeout";
static NSString *const kBraveWarmScript =
    @"with timeout of 3.0 seconds\n"
     "tell application \"Brave Browser\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "if (mode of front window) is \"incognito\" then return \"private\"\n"
     "return URL of active tab of front window\n"
     "end tell\n"
     "end timeout";
static NSString *const kSafariWarmScript =
    @"with timeout of 3.0 seconds\n"
     "tell application \"Safari\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "return URL of current tab of front window\n"
     "end tell\n"
     "end timeout";

// Close scripts for the focus guard. Constant like the others; the caller
// checks the front tab's host with the URL script first.
static NSString *const kCloseChromeScript =
    @"with timeout of 1.0 seconds\n"
     "tell application \"Google Chrome\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "close active tab of front window\n"
     "return \"ok\"\n"
     "end tell\n"
     "end timeout";
static NSString *const kCloseArcScript =
    @"with timeout of 1.0 seconds\n"
     "tell application \"Arc\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "close active tab of front window\n"
     "return \"ok\"\n"
     "end tell\n"
     "end timeout";
static NSString *const kCloseBraveScript =
    @"with timeout of 1.0 seconds\n"
     "tell application \"Brave Browser\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "close active tab of front window\n"
     "return \"ok\"\n"
     "end tell\n"
     "end timeout";
static NSString *const kCloseSafariScript =
    @"with timeout of 1.0 seconds\n"
     "tell application \"Safari\"\n"
     "if (count of windows) is 0 then return \"\"\n"
     "close current tab of front window\n"
     "return \"ok\"\n"
     "end tell\n"
     "end timeout";

// Self-test scripts exercise the same execution path without a browser.
static NSString *const kSelftestURLScript =
    @"return \"https://user:pw@Example.COM:8080/a/b/?q=1#f\"";
static NSString *const kSelftestErrorScript =
    @"error \"selftest\" number -1712";

#define TOCK_SCRIPT_SLOTS 20

// slot maps (kind, warm) to a cache index, or -1 for an unknown kind.
static int tock_script_slot(int kind, int warm) {
    int base;
    switch (kind) {
    case TOCK_SCRIPT_CHROME: base = 0; break;
    case TOCK_SCRIPT_ARC: base = 1; break;
    case TOCK_SCRIPT_BRAVE: base = 2; break;
    case TOCK_SCRIPT_SAFARI: base = 3; break;
    case TOCK_SCRIPT_SELFTEST_URL: base = 4; break;
    case TOCK_SCRIPT_SELFTEST_ERROR: base = 5; break;
    case TOCK_SCRIPT_CLOSE_CHROME: base = 6; break;
    case TOCK_SCRIPT_CLOSE_ARC: base = 7; break;
    case TOCK_SCRIPT_CLOSE_BRAVE: base = 8; break;
    case TOCK_SCRIPT_CLOSE_SAFARI: base = 9; break;
    default: return -1;
    }
    return base * 2 + (warm ? 1 : 0);
}

static NSString *tock_script_source(int kind, int warm) {
    switch (kind) {
    case TOCK_SCRIPT_CHROME: return warm ? kChromeWarmScript : kChromeScript;
    case TOCK_SCRIPT_ARC: return warm ? kArcWarmScript : kArcScript;
    case TOCK_SCRIPT_BRAVE: return warm ? kBraveWarmScript : kBraveScript;
    case TOCK_SCRIPT_SAFARI: return warm ? kSafariWarmScript : kSafariScript;
    case TOCK_SCRIPT_SELFTEST_URL: return kSelftestURLScript;
    case TOCK_SCRIPT_SELFTEST_ERROR: return kSelftestErrorScript;
    case TOCK_SCRIPT_CLOSE_CHROME: return kCloseChromeScript;
    case TOCK_SCRIPT_CLOSE_ARC: return kCloseArcScript;
    case TOCK_SCRIPT_CLOSE_BRAVE: return kCloseBraveScript;
    case TOCK_SCRIPT_CLOSE_SAFARI: return kCloseSafariScript;
    }
    return nil;
}

// Touched only on the main thread.
static NSAppleScript *tock_scripts[TOCK_SCRIPT_SLOTS];

static int tock_error_number(NSDictionary *err) {
    NSNumber *n = err[NSAppleScriptErrorNumber];
    return n != nil ? n.intValue : -1;
}

static int tock_run_script_main(int kind, int warm, char *out, int outlen, int *errnum) {
    out[0] = '\0';
    *errnum = 0;
    int slot = tock_script_slot(kind, warm);
    if (slot < 0) {
        *errnum = -1;
        return 1;
    }
    NSAppleScript *script = tock_scripts[slot];
    if (script == nil) {
        script = [[NSAppleScript alloc] initWithSource:tock_script_source(kind, warm)];
        NSDictionary *cerr = nil;
        if (script == nil || ![script compileAndReturnError:&cerr]) {
            *errnum = tock_error_number(cerr);
            return 1;
        }
        tock_scripts[slot] = script;
    }
    NSDictionary *err = nil;
    NSAppleEventDescriptor *res = [script executeAndReturnError:&err];
    if (res == nil) {
        *errnum = tock_error_number(err);
        return 1;
    }
    NSString *s = res.stringValue;
    if (s != nil) {
        const char *u = [s UTF8String];
        if (u != NULL) {
            strlcpy(out, u, (size_t)outlen);
        }
    }
    return 0;
}

int tock_run_script(int kind, int warm, char *out, int outlen, int *errnum) {
    if ([NSThread isMainThread]) {
        return tock_run_script_main(kind, warm, out, outlen, errnum);
    }
    // Only tests call from another thread; the main thread pumps its run loop.
    __block int rc = 1;
    dispatch_sync(dispatch_get_main_queue(), ^{
        rc = tock_run_script_main(kind, warm, out, outlen, errnum);
    });
    return rc;
}

void tock_automation_check(const char *bundle, int ask) {
    char *copy = strdup(bundle);
    if (copy == NULL) {
        return;
    }
    // AEDeterminePermissionToAutomateTarget blocks (and may show a prompt), so
    // it never runs on the main thread (T-1-mainblock).
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
        AEAddressDesc desc;
        OSStatus status = -1744;
        if (AECreateDesc(typeApplicationBundleID, copy, strlen(copy), &desc) == noErr) {
            status = AEDeterminePermissionToAutomateTarget(&desc, typeWildCard, typeWildCard,
                                                           ask ? true : false);
            AEDisposeDesc(&desc);
        }
        goAutomationResult(copy, (int)status);
        free(copy);
    });
}

static int tock_hide_app_main(NSString *bundle) {
    int n = 0;
    for (NSRunningApplication *app in [NSRunningApplication runningApplicationsWithBundleIdentifier:bundle]) {
        if ([app hide]) {
            n++;
        }
    }
    return n;
}

int tock_hide_app(const char *bundle) {
    NSString *b = [NSString stringWithUTF8String:bundle];
    if (b == nil) {
        return 0;
    }
    if ([NSThread isMainThread]) {
        return tock_hide_app_main(b);
    }
    __block int n = 0;
    dispatch_sync(dispatch_get_main_queue(), ^{
        n = tock_hide_app_main(b);
    });
    return n;
}
