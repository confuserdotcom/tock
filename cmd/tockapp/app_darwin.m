#import <Cocoa/Cocoa.h>
#include "app_darwin.h"

static NSString *str(const char *s) { return s ? [NSString stringWithUTF8String:s] : @""; }

static NSInteger say(NSString *title, NSString *text, NSString *ok, NSString *cancel, NSAlertStyle style) {
    NSAlert *a = [NSAlert new];
    a.messageText = title;
    a.informativeText = text;
    a.alertStyle = style;
    [a addButtonWithTitle:ok];
    if (cancel) {
        [a addButtonWithTitle:cancel];
    }
    [NSApp activateIgnoringOtherApps:YES];
    return [a runModal];
}

// lastLines is the end of a command's output, for a failure to show.
static NSString *lastLines(NSString *s, NSUInteger n) {
    NSArray *ls = [[s stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet] componentsSeparatedByString:@"\n"];
    if (ls.count > n) {
        ls = [ls subarrayWithRange:NSMakeRange(ls.count - n, n)];
    }
    return [ls componentsJoinedByString:@"\n"];
}

// working is a small window that says setup is running.
static NSWindow *working(void) {
    NSWindow *w = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 340, 96)
                                              styleMask:NSWindowStyleMaskTitled
                                                backing:NSBackingStoreBuffered
                                                  defer:NO];
    w.title = @"tock";
    w.releasedWhenClosed = NO;
    NSProgressIndicator *spin = [[NSProgressIndicator alloc] initWithFrame:NSMakeRect(24, 40, 18, 18)];
    spin.style = NSProgressIndicatorStyleSpinning;
    spin.controlSize = NSControlSizeSmall;
    [spin startAnimation:nil];
    NSTextField *l = [NSTextField labelWithString:@"Setting up tock…"];
    l.font = [NSFont systemFontOfSize:13 weight:NSFontWeightMedium];
    l.frame = NSMakeRect(52, 48, 270, 18);
    NSTextField *s = [NSTextField labelWithString:@"macOS may ask for your password."];
    s.font = [NSFont systemFontOfSize:12];
    s.textColor = NSColor.secondaryLabelColor;
    s.frame = NSMakeRect(52, 28, 270, 16);
    [w.contentView addSubview:spin];
    [w.contentView addSubview:l];
    [w.contentView addSubview:s];
    [w center];
    return w;
}

int launcherRun(const char *kind, const char *title, const char *text, const char *ok, char **argv, int argc) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        NSString *k = str(kind);
        NSMutableArray<NSString *> *args = [NSMutableArray new];
        for (int i = 1; i < argc; i++) {
            [args addObject:str(argv[i])];
        }
        if ([k isEqualToString:@"stop"]) {
            say(str(title), str(text), @"OK", nil, NSAlertStyleWarning);
            return 1;
        }
        if (argc < 1) {
            return 1;
        }
        NSTask *t = [NSTask new];
        t.executableURL = [NSURL fileURLWithPath:str(argv[0])];
        t.arguments = args;
        if ([k isEqualToString:@"open"]) {
            NSError *err = nil;
            if (![t launchAndReturnError:&err]) {
                return 1;
            }
            [t waitUntilExit];
            return t.terminationStatus;
        }
        if (say(str(title), str(text), str(ok), @"Quit", NSAlertStyleInformational) != NSAlertFirstButtonReturn) {
            return 0;
        }
        NSPipe *pipe = [NSPipe pipe];
        t.standardOutput = pipe;
        t.standardError = pipe;
        NSMutableData *outData = [NSMutableData new];
        pipe.fileHandleForReading.readabilityHandler = ^(NSFileHandle *h) {
            NSData *d = h.availableData;
            @synchronized(outData) {
                [outData appendData:d];
            }
        };
        __block int status = 1;
        NSWindow *w = working();
        t.terminationHandler = ^(NSTask *done) {
            dispatch_async(dispatch_get_main_queue(), ^{
                pipe.fileHandleForReading.readabilityHandler = nil;
                NSData *rest = [pipe.fileHandleForReading readDataToEndOfFile];
                NSString *out;
                @synchronized(outData) {
                    [outData appendData:rest];
                    out = [[NSString alloc] initWithData:outData encoding:NSUTF8StringEncoding] ?: @"";
                }
                status = done.terminationStatus;
                [w orderOut:nil];
                if (status == 0) {
                    say(@"tock is running",
                        @"Look for its clock in the menu bar. If a permission is still missing, System Settings is open at it: turn tock on there.",
                        @"OK", nil, NSAlertStyleInformational);
                } else {
                    say(@"Setup didn't finish", lastLines(out, 12), @"OK", nil, NSAlertStyleWarning);
                }
                [NSApp stop:nil];
                // stop takes effect after the next event
                [NSApp postEvent:[NSEvent otherEventWithType:NSEventTypeApplicationDefined
                                                    location:NSZeroPoint
                                               modifierFlags:0
                                                   timestamp:0
                                                windowNumber:0
                                                     context:nil
                                                     subtype:0
                                                       data1:0
                                                       data2:0]
                         atStart:NO];
            });
        };
        NSError *err = nil;
        if (![t launchAndReturnError:&err]) {
            say(@"Setup didn't start", err.localizedDescription ?: @"", @"OK", nil, NSAlertStyleWarning);
            return 1;
        }
        [w makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
        [NSApp run];
        return status;
    }
}
