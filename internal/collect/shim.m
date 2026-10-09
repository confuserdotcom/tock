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
