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

// tock_ax_init bounds every AX call to 0.25 s (a hung target then costs at
// most that much main-thread time) and, when asked and untrusted, shows the
// system Accessibility prompt. Both happen at most once per process.
void tock_ax_init(int prompt) {
    static int timeout_set = 0;
    static int prompted = 0;
    if (!timeout_set) {
        timeout_set = 1;
        AXUIElementRef sys = AXUIElementCreateSystemWide();
        if (sys != NULL) {
            AXUIElementSetMessagingTimeout(sys, 0.25f);
            CFRelease(sys);
        }
    }
    if (prompt && !prompted && !AXIsProcessTrusted()) {
        prompted = 1;
        NSDictionary *opts = @{(__bridge NSString *)kAXTrustedCheckOptionPrompt : @YES};
        AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)opts);
    }
}

// Settle delay between an activation notification and the sample read (D-04):
// the window of the newly active app has not focused yet at notification time.
#define TOCK_ACTIVATION_SETTLE_MS 150

static long long tock_now_ms(void) {
    return (long long)([[NSDate date] timeIntervalSince1970] * 1000.0);
}

// tock_observe_event forwards a notification as a hard-boundary event stamped
// with its receipt time. Codes: 1 sleep, 2 wake, 3 lock, 4 unlock. Duplicates
// are fine: the merger treats them idempotently.
static void tock_observe_event(NSNotificationCenter *center, NSString *name, int code) {
    [center addObserverForName:name
                        object:nil
                         queue:nil
                    usingBlock:^(NSNotification *n) {
        (void)n;
        goEvent(code, tock_now_ms());
    }];
}

void tock_start_observers(void) {
    NSNotificationCenter *wc = [[NSWorkspace sharedWorkspace] notificationCenter];
    [wc addObserverForName:NSWorkspaceDidActivateApplicationNotification
                    object:nil
                     queue:nil
                usingBlock:^(NSNotification *n) {
        // Stamp the receipt time first: this is the moment of the switch.
        long long ts = tock_now_ms();
        NSRunningApplication *app = n.userInfo[NSWorkspaceApplicationKey];
        NSString *bundleID = app.bundleIdentifier;
        NSString *name = app.localizedName;
        // Read the window only after it has settled; the main queue is drained
        // by CFRunLoopRun on this same thread.
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW,
                                     (int64_t)TOCK_ACTIVATION_SETTLE_MS * NSEC_PER_MSEC),
                       dispatch_get_main_queue(), ^{
            char bundle[256];
            char appName[256];
            tock_copy(bundle, sizeof bundle, bundleID);
            tock_copy(appName, sizeof appName, name);
            goActivation(bundle, appName, ts);
        });
    }];

    tock_observe_event(wc, NSWorkspaceWillSleepNotification, 1);
    tock_observe_event(wc, NSWorkspaceDidWakeNotification, 2);
    tock_observe_event(wc, NSWorkspaceSessionDidResignActiveNotification, 3);
    tock_observe_event(wc, NSWorkspaceSessionDidBecomeActiveNotification, 4);

    // Block API for now; the plan 01-10 spike decides whether these need the
    // selector API with NSNotificationSuspensionBehaviorDeliverImmediately.
    NSDistributedNotificationCenter *dc = [NSDistributedNotificationCenter defaultCenter];
    tock_observe_event(dc, @"com.apple.screenIsLocked", 3);
    tock_observe_event(dc, @"com.apple.screenIsUnlocked", 4);
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

// tock_locked reads the console lock flag from the session dictionary. A
// missing key (or missing dictionary) means unlocked.
static int tock_locked(void) {
    int locked = 0;
    CFDictionaryRef d = CGSessionCopyCurrentDictionary();
    if (d != NULL) {
        CFTypeRef v = CFDictionaryGetValue(d, CFSTR("CGSSessionScreenIsLocked"));
        if (v != NULL && CFGetTypeID(v) == CFBooleanGetTypeID()) {
            locked = CFBooleanGetValue((CFBooleanRef)v) ? 1 : 0;
        }
        CFRelease(d);
    }
    return locked;
}

// tock_focused_title copies the focused window title of pid into out. On any
// AX failure the code is stored in *ax_err and has_title stays 0.
static void tock_focused_title(pid_t pid, tock_poll_t *out) {
    AXUIElementRef appEl = AXUIElementCreateApplication(pid);
    if (appEl == NULL) {
        return;
    }
    CFTypeRef win = NULL;
    AXError err = AXUIElementCopyAttributeValue(appEl, kAXFocusedWindowAttribute, &win);
    if (err != kAXErrorSuccess || win == NULL) {
        out->ax_err = (int)err;
        if (win != NULL) {
            CFRelease(win);
        }
        CFRelease(appEl);
        return;
    }
    CFTypeRef title = NULL;
    err = AXUIElementCopyAttributeValue((AXUIElementRef)win, kAXTitleAttribute, &title);
    if (err == kAXErrorSuccess && title != NULL && CFGetTypeID(title) == CFStringGetTypeID()) {
        if (CFStringGetCString((CFStringRef)title, out->title, sizeof out->title,
                               kCFStringEncodingUTF8)) {
            out->has_title = 1;
        } else {
            // Longer than the buffer or not encodable: keep a truncated copy.
            tock_copy(out->title, sizeof out->title, (__bridge NSString *)title);
            out->has_title = 1;
        }
    } else if (err != kAXErrorSuccess) {
        out->ax_err = (int)err;
    }
    if (title != NULL) {
        CFRelease(title);
    }
    CFRelease(win);
    CFRelease(appEl);
}

void tock_poll(tock_poll_t *out) {
    memset(out, 0, sizeof *out);
    out->ax_trusted = AXIsProcessTrusted() ? 1 : 0;
    out->idle_s = CGEventSourceSecondsSinceLastEventType(kCGEventSourceStateHIDSystemState,
                                                         kCGAnyInputEventType);
    out->locked = tock_locked();
    NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
    if (app == nil) {
        return;
    }
    tock_copy(out->bundle, sizeof out->bundle, app.bundleIdentifier);
    tock_copy(out->app, sizeof out->app, app.localizedName);
    if (out->ax_trusted && app.processIdentifier > 0) {
        tock_focused_title(app.processIdentifier, out);
    }
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
