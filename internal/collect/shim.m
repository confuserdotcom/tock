// Objective-C shim for the macOS collector. Every function here runs on the
// process main thread (locked by runtime.LockOSThread in collect_darwin.go).
// Do not create an NSApplication: activation notifications arrive without it.
#import <Cocoa/Cocoa.h>
#include <string.h>

#include "_cgo_export.h"
#include "shim.h"

static void tock_copy(char *dst, size_t n, NSString *s) {
    if (s == nil) {
        dst[0] = '\0';
        return;
    }
    const char *u = [s UTF8String];
    if (u == NULL) {
        dst[0] = '\0';
        return;
    }
    strlcpy(dst, u, n);
}

void tock_start_observers(void) {
    NSNotificationCenter *wc = [[NSWorkspace sharedWorkspace] notificationCenter];
    [wc addObserverForName:NSWorkspaceDidActivateApplicationNotification
                    object:nil
                     queue:nil
                usingBlock:^(NSNotification *n) {
        // Stamp the receipt time first: this is the moment of the switch.
        long long ts = (long long)([[NSDate date] timeIntervalSince1970] * 1000.0);
        NSRunningApplication *app = n.userInfo[NSWorkspaceApplicationKey];
        char bundle[256];
        char name[256];
        tock_copy(bundle, sizeof bundle, app.bundleIdentifier);
        tock_copy(name, sizeof name, app.localizedName);
        goActivation(bundle, name, ts);
    }];
}

static void tock_timer_fired(CFRunLoopTimerRef timer, void *info) {
    (void)timer;
    (void)info;
    goPollTick();
}

void tock_start_poll_timer(double seconds) {
    CFRunLoopTimerRef t = CFRunLoopTimerCreate(NULL,
                                               CFAbsoluteTimeGetCurrent() + seconds,
                                               seconds, 0, 0,
                                               tock_timer_fired, NULL);
    CFRunLoopAddTimer(CFRunLoopGetMain(), t, kCFRunLoopCommonModes);
    CFRelease(t);
}

void tock_poll(tock_poll_t *out) {
    memset(out, 0, sizeof *out);
    NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
    if (app == nil) {
        return;
    }
    tock_copy(out->bundle, sizeof out->bundle, app.bundleIdentifier);
    tock_copy(out->app, sizeof out->app, app.localizedName);
}

void tock_run_loop(void) {
    CFRunLoopRun();
}

void tock_run_loop_slice(double seconds) {
    CFRunLoopRunInMode(kCFRunLoopDefaultMode, seconds, false);
}

void tock_stop_run_loop(void) {
    CFRunLoopStop(CFRunLoopGetMain());
}
