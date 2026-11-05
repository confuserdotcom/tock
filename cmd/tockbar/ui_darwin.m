// The Cocoa side of the tock menu bar app. Every UI call hops to the main
// thread, which runs [NSApp run]; clicks go back to Go through goMenu and
// goResponse, which only queue them.
#import <Carbon/Carbon.h>
#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>

#include "_cgo_export.h"
#include "ui_darwin.h"

static NSString *str(const char *s) {
    NSString *v = s ? [NSString stringWithUTF8String:s] : nil;
    return v ? v : @"";
}

@interface TockBar : NSObject <NSApplicationDelegate, UNUserNotificationCenterDelegate>
@property(strong) NSStatusItem *item;
@end

static TockBar *bar;

@implementation TockBar
// tock:// links (Shortcuts, Raycast) arrive as Apple events; register
// before launch ends so a link that launched the app is not lost.
- (void)applicationWillFinishLaunching:(NSNotification *)n {
    (void)n;
    [[NSAppleEventManager sharedAppleEventManager] setEventHandler:self
                                                       andSelector:@selector(openURL:reply:)
                                                     forEventClass:kInternetEventClass
                                                        andEventID:kAEGetURL];
}

- (void)openURL:(NSAppleEventDescriptor *)e reply:(NSAppleEventDescriptor *)r {
    (void)r;
    NSString *u = [[e paramDescriptorForKeyword:keyDirectObject] stringValue];
    if (u != nil) {
        goMenu((char *)[[@"url:" stringByAppendingString:u] UTF8String]);
    }
}

- (void)applicationDidFinishLaunching:(NSNotification *)n {
    (void)n;
    self.item = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
    self.item.button.title = @"◷";
    self.item.menu = [NSMenu new];
    [UNUserNotificationCenter currentNotificationCenter].delegate = self;
    goReady();
}

- (void)clicked:(NSMenuItem *)m {
    goMenu((char *)[m.representedObject UTF8String]);
}

// Show banners even while the app counts as frontmost.
- (void)userNotificationCenter:(UNUserNotificationCenter *)c
       willPresentNotification:(UNNotification *)n
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))h {
    (void)c;
    (void)n;
    h(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList |
      UNNotificationPresentationOptionSound);
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)c
    didReceiveNotificationResponse:(UNNotificationResponse *)r
             withCompletionHandler:(void (^)(void))h {
    (void)c;
    NSNumber *nid = r.notification.request.content.userInfo[@"notice"];
    if (nid != nil) {
        goResponse([nid longLongValue], (char *)[r.actionIdentifier UTF8String]);
    }
    h();
}
@end

void bar_run(void) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        [app setActivationPolicy:NSApplicationActivationPolicyAccessory];
        bar = [TockBar new];
        app.delegate = bar;
        [app run];
    }
}

void bar_set_title(const char *title) {
    NSString *t = str(title);
    dispatch_async(dispatch_get_main_queue(), ^{
        bar.item.button.title = t;
    });
}

// kindColor is DESIGN.md's colour of a kind, darker on a light menu and
// lighter on a dark one; an unknown kind is tertiary grey.
static NSColor *kindColor(NSString *k) {
    static NSDictionary<NSString *, NSColor *> *m;
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        NSColor * (^hex)(unsigned) = ^(unsigned v) {
            return [NSColor colorWithSRGBRed:((v >> 16) & 0xff) / 255.0
                                       green:((v >> 8) & 0xff) / 255.0
                                        blue:(v & 0xff) / 255.0
                                       alpha:1];
        };
        NSColor * (^pair)(unsigned, unsigned) = ^(unsigned light, unsigned dark) {
            NSColor *l = hex(light), *d = hex(dark);
            return [NSColor colorWithName:nil
                          dynamicProvider:^NSColor *(NSAppearance *a) {
                              NSAppearanceName n = [a bestMatchFromAppearancesWithNames:@[ NSAppearanceNameAqua, NSAppearanceNameDarkAqua ]];
                              return [n isEqualToString:NSAppearanceNameDarkAqua] ? d : l;
                          }];
        };
        m = @{
            @"deep" : pair(0x2d7a68, 0x63bfa8),
            @"shallow" : pair(0x5a78a3, 0x8fa9d1),
            @"neutral" : pair(0x858991, 0x7d828c),
            @"browse" : pair(0x9a6585, 0xc993b4),
            @"distract" : pair(0xb5462c, 0xe57d62),
            @"none" : pair(0xb0aea6, 0x56554f),
        };
    });
    NSColor *c = m[k];
    return c ? c : [NSColor tertiaryLabelColor];
}

// stripTitle draws kinds ("k1,k2,…") as one coloured block each.
static NSAttributedString *stripTitle(NSString *kinds) {
    NSMutableAttributedString *s = [NSMutableAttributedString new];
    NSFont *font = [NSFont monospacedSystemFontOfSize:13 weight:NSFontWeightRegular];
    for (NSString *k in [kinds componentsSeparatedByString:@","]) {
        NSDictionary *attrs = @{NSForegroundColorAttributeName : kindColor(k), NSFontAttributeName : font};
        [s appendAttributedString:[[NSAttributedString alloc] initWithString:@"▮" attributes:attrs]];
    }
    return s;
}

// spec: one line per item, "id\ttitle[\tc]"; an empty id is a disabled label,
// a line "-" a separator and a third field "c" a check mark. A fourth field
// "s:k1,k2,…" makes the row a colour strip, one block per kind. Each leading ">"
// is one level of submenu: the rows deeper than the item above them are its
// submenu.
void bar_set_menu(const char *spec) {
    NSString *s = str(spec);
    dispatch_async(dispatch_get_main_queue(), ^{
        NSMenu *m = [NSMenu new];
        m.autoenablesItems = NO;
        NSMutableArray<NSMenu *> *stack = [NSMutableArray arrayWithObject:m];
        for (NSString *raw in [s componentsSeparatedByString:@"\n"]) {
            NSString *line = raw;
            NSUInteger depth = 0;
            while ([line hasPrefix:@">"]) {
                line = [line substringFromIndex:1];
                depth++;
            }
            if (depth > stack.count) {
                continue;
            }
            if (depth == stack.count) {
                NSMenuItem *parent = stack.lastObject.itemArray.lastObject;
                if (parent == nil) {
                    continue;
                }
                NSMenu *submenu = [NSMenu new];
                submenu.autoenablesItems = NO;
                parent.submenu = submenu;
                parent.enabled = YES;
                [stack addObject:submenu];
            }
            while (stack.count > depth + 1) {
                [stack removeLastObject];
            }
            NSMenu *top = stack.lastObject;
            if ([line isEqualToString:@"-"]) {
                [top addItem:[NSMenuItem separatorItem]];
                continue;
            }
            NSArray *f = [line componentsSeparatedByString:@"\t"];
            if (f.count < 2) {
                continue;
            }
            if (f.count >= 4 && [f[3] hasPrefix:@"s:"]) {
                // Enabled with no action: a disabled item would grey the colours.
                NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:@"" action:nil keyEquivalent:@""];
                it.attributedTitle = stripTitle([f[3] substringFromIndex:2]);
                it.enabled = YES;
                [top addItem:it];
                continue;
            }
            NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:f[1] action:nil keyEquivalent:@""];
            if ([f[0] length] > 0) {
                it.target = bar;
                it.action = @selector(clicked:);
                it.representedObject = f[0];
                it.enabled = YES;
            } else {
                it.enabled = NO;
            }
            if (f.count >= 3 && [f[2] isEqualToString:@"c"]) {
                it.state = NSControlStateValueOn;
            }
            [top addItem:it];
        }
        bar.item.menu = m;
    });
}

// categories: one line per category, "id\taction\ttitle\taction\ttitle…".
// They are set again before every post so the new one has its buttons.
void bar_post(long long nid, const char *title, const char *body, const char *category, const char *categories) {
    NSString *t = str(title), *b = str(body), *cat = str(category), *cats = str(categories);
    dispatch_async(dispatch_get_main_queue(), ^{
        UNUserNotificationCenter *c = [UNUserNotificationCenter currentNotificationCenter];
        NSMutableSet *set = [NSMutableSet set];
        for (NSString *line in [cats componentsSeparatedByString:@"\n"]) {
            NSArray *f = [line componentsSeparatedByString:@"\t"];
            if (f.count < 3) {
                continue;
            }
            NSMutableArray *acts = [NSMutableArray array];
            for (NSUInteger i = 1; i + 1 < f.count; i += 2) {
                [acts addObject:[UNNotificationAction actionWithIdentifier:f[i] title:f[i + 1] options:0]];
            }
            [set addObject:[UNNotificationCategory categoryWithIdentifier:f[0]
                                                                  actions:acts
                                                        intentIdentifiers:@[]
                                                                  options:0]];
        }
        [c setNotificationCategories:set];
        UNMutableNotificationContent *ct = [UNMutableNotificationContent new];
        ct.title = t;
        ct.body = b;
        ct.categoryIdentifier = cat;
        ct.userInfo = @{@"notice" : @(nid)};
        ct.sound = [UNNotificationSound defaultSound];
        NSString *rid = [NSString stringWithFormat:@"tock-%lld", nid];
        [c addNotificationRequest:[UNNotificationRequest requestWithIdentifier:rid content:ct trigger:nil]
            withCompletionHandler:^(NSError *e) {
                if (e != nil) {
                    goPostFailed(nid, (char *)[e.localizedDescription UTF8String]);
                }
            }];
    });
}

static int access_of(UNAuthorizationStatus s) {
    switch (s) {
    case UNAuthorizationStatusAuthorized:
    case UNAuthorizationStatusProvisional:
        return 1;
    case UNAuthorizationStatusDenied:
        return 2;
    default:
        return 0;
    }
}

// bar_check_auth reports the notification access through goAuth. With
// prompt set and the access undetermined, it asks the user first.
void bar_check_auth(int prompt) {
    UNUserNotificationCenter *c = [UNUserNotificationCenter currentNotificationCenter];
    [c getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s) {
        if (prompt && s.authorizationStatus == UNAuthorizationStatusNotDetermined) {
            [c requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                             completionHandler:^(BOOL ok, NSError *e) {
                                 (void)e;
                                 goAuth(ok ? 1 : 2);
                             }];
            return;
        }
        goAuth(access_of(s.authorizationStatus));
    }];
}

void bar_open_url(const char *url) {
    NSString *u = str(url);
    dispatch_async(dispatch_get_main_queue(), ^{
        [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:u]];
    });
}

void bar_quit(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp terminate:nil];
    });
}

// ---------------------------------------------------------------------------
// Break screen: a dimmed window over every screen, the break on the main one.
// It never locks anything: Esc or a button closes it.
// ---------------------------------------------------------------------------

static NSColor *rgb(int hex) {
    return [NSColor colorWithSRGBRed:((hex >> 16) & 0xff) / 255.0
                               green:((hex >> 8) & 0xff) / 255.0
                                blue:(hex & 0xff) / 255.0
                               alpha:1];
}

// CurtainBar draws the break: brass for the planned part, rust for the overrun.
@interface CurtainBar : NSView
@property double planned; // share of the bar the planned break takes, 0..1
@end

@implementation CurtainBar
- (void)drawRect:(NSRect)r {
    (void)r;
    NSRect b = self.bounds;
    CGFloat rad = b.size.height / 2;
    NSBezierPath *track = [NSBezierPath bezierPathWithRoundedRect:b xRadius:rad yRadius:rad];
    [rgb(0x2b241b) setFill];
    [track fill];
    [NSGraphicsContext saveGraphicsState];
    [track addClip];
    CGFloat split = b.size.width * MAX(0, MIN(1, self.planned));
    [rgb(0xe3a446) setFill];
    NSRectFill(NSMakeRect(b.origin.x, b.origin.y, split, b.size.height));
    [rgb(0xb5462c) setFill];
    NSRectFill(NSMakeRect(b.origin.x + split, b.origin.y, b.size.width - split, b.size.height));
    [NSGraphicsContext restoreGraphicsState];
}
@end

@interface CurtainWindow : NSWindow
@end

@interface Curtain : NSObject
@property long long nid;
@property long long fromMS, untilMS;
@property(strong) NSString *word; // after the clock: "away", "off task"
@property(strong) NSArray<NSString *> *actions;
@property(strong) NSMutableArray<NSWindow *> *windows;
@property(strong) NSTextField *clock;
@property(strong) CurtainBar *bar;
@property(strong) NSTimer *timer;
- (void)close;
@end

static Curtain *curtain;

@implementation CurtainWindow
- (BOOL)canBecomeKeyWindow {
    return YES;
}
- (void)cancelOperation:(id)sender {
    (void)sender;
    [curtain close];
}
- (void)keyDown:(NSEvent *)e {
    if (e.keyCode == 53) { // Esc
        [curtain close];
        return;
    }
    [super keyDown:e];
}
@end

static NSTextField *curtainLabel(NSString *s, NSFont *f, NSColor *c) {
    NSTextField *l = [NSTextField labelWithString:s];
    l.font = f;
    l.textColor = c;
    l.alignment = NSTextAlignmentCenter;
    l.lineBreakMode = NSLineBreakByWordWrapping;
    l.maximumNumberOfLines = 3;
    l.preferredMaxLayoutWidth = 520;
    return l;
}

@implementation Curtain
- (void)tick {
    long long now = (long long)([[NSDate date] timeIntervalSince1970] * 1000);
    long long away = MAX(0, now - self.fromMS) / 1000;
    NSString *t = away >= 3600
                      ? [NSString stringWithFormat:@"%lldh %02lld %@", away / 3600, (away % 3600) / 60, self.word]
                      : [NSString stringWithFormat:@"%lld:%02lld %@", away / 60, away % 60, self.word];
    self.clock.stringValue = t;
    long long whole = MAX(now, self.untilMS) - self.fromMS;
    self.bar.planned = whole > 0 ? (double)(self.untilMS - self.fromMS) / (double)whole : 1;
    [self.bar setNeedsDisplay:YES];
}

- (void)clicked:(NSButton *)b {
    NSInteger i = b.tag;
    long long nid = self.nid;
    NSString *act = i >= 0 && i < (NSInteger)self.actions.count ? self.actions[i] : nil;
    [self close];
    if (act != nil) {
        goResponse(nid, (char *)[act UTF8String]);
    }
}

- (void)close {
    [self.timer invalidate];
    self.timer = nil;
    for (NSWindow *w in self.windows) {
        [w orderOut:nil];
    }
    [self.windows removeAllObjects];
    if (curtain == self) {
        curtain = nil;
    }
}

- (NSView *)contentWithTitle:(NSString *)title body:(NSString *)body buttons:(NSArray<NSString *> *)f {
    NSTextField *kicker = curtainLabel([title uppercaseString],
                                       [NSFont monospacedSystemFontOfSize:12 weight:NSFontWeightMedium], rgb(0x8a7d68));
    self.clock = curtainLabel(@"", [NSFont monospacedDigitSystemFontOfSize:72 weight:NSFontWeightBold], rgb(0xf1e4cf));
    self.bar = [CurtainBar new];
    self.bar.translatesAutoresizingMaskIntoConstraints = NO;
    [self.bar.widthAnchor constraintEqualToConstant:360].active = YES;
    [self.bar.heightAnchor constraintEqualToConstant:6].active = YES;
    NSTextField *msg = curtainLabel(body, [NSFont systemFontOfSize:15], rgb(0xc7b79c));
    NSMutableArray *acts = [NSMutableArray array];
    NSStackView *row = [NSStackView new];
    row.orientation = NSUserInterfaceLayoutOrientationHorizontal;
    row.spacing = 10;
    for (NSUInteger i = 0; i + 1 < f.count; i += 2) {
        NSButton *b = [NSButton buttonWithTitle:f[i + 1] target:self action:@selector(clicked:)];
        b.tag = (NSInteger)acts.count;
        b.bezelStyle = NSBezelStyleRounded;
        b.controlSize = NSControlSizeLarge;
        if (acts.count == 0) {
            b.keyEquivalent = @"\r"; // the first answer is the default
            b.bezelColor = rgb(0xe3a446);
        }
        [acts addObject:f[i]];
        [row addArrangedSubview:b];
    }
    self.actions = acts;
    NSTextField *hint = curtainLabel(@"esc to dismiss · never locks your Mac",
                                     [NSFont monospacedSystemFontOfSize:11 weight:NSFontWeightRegular], rgb(0x8a7d68));
    NSStackView *col = [NSStackView stackViewWithViews:@[ kicker, self.clock, self.bar, msg, row, hint ]];
    col.orientation = NSUserInterfaceLayoutOrientationVertical;
    col.alignment = NSLayoutAttributeCenterX;
    col.spacing = 18;
    [col setCustomSpacing:28 afterView:msg];
    col.translatesAutoresizingMaskIntoConstraints = NO;
    return col;
}
@end

void bar_curtain(long long nid, const char *title, const char *body, const char *word, long long from_ms,
                 long long until_ms, const char *buttons) {
    NSString *t = str(title), *b = str(body), *wd = str(word), *bs = str(buttons);
    dispatch_async(dispatch_get_main_queue(), ^{
        [curtain close];
        Curtain *c = [Curtain new];
        c.nid = nid;
        c.fromMS = from_ms;
        c.untilMS = until_ms;
        c.word = wd.length > 0 ? wd : @"away";
        c.windows = [NSMutableArray array];
        NSArray<NSString *> *f = bs.length > 0 ? [bs componentsSeparatedByString:@"\t"] : @[];
        BOOL still = [[NSWorkspace sharedWorkspace] accessibilityDisplayShouldReduceMotion];
        NSScreen *main = [NSScreen mainScreen];
        for (NSScreen *s in [NSScreen screens]) {
            CurtainWindow *w = [[CurtainWindow alloc] initWithContentRect:s.frame
                                                                styleMask:NSWindowStyleMaskBorderless
                                                                  backing:NSBackingStoreBuffered
                                                                    defer:NO];
            w.releasedWhenClosed = NO;
            w.level = NSMainMenuWindowLevel + 2;
            w.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces |
                                   NSWindowCollectionBehaviorFullScreenAuxiliary |
                                   NSWindowCollectionBehaviorStationary | NSWindowCollectionBehaviorIgnoresCycle;
            w.opaque = NO;
            w.backgroundColor = [rgb(0x0d0b09) colorWithAlphaComponent:0.94];
            w.hasShadow = NO;
            [w setFrame:s.frame display:NO];
            if (s == main) {
                NSView *v = [c contentWithTitle:t body:b buttons:f];
                [w.contentView addSubview:v];
                [v.centerXAnchor constraintEqualToAnchor:w.contentView.centerXAnchor].active = YES;
                [v.centerYAnchor constraintEqualToAnchor:w.contentView.centerYAnchor].active = YES;
            }
            w.alphaValue = still ? 1 : 0;
            [c.windows addObject:w];
        }
        curtain = c;
        [c tick];
        c.timer = [NSTimer scheduledTimerWithTimeInterval:1 target:c selector:@selector(tick) userInfo:nil repeats:YES];
        [NSApp activateIgnoringOtherApps:YES];
        for (NSWindow *w in c.windows) {
            [w orderFrontRegardless];
            if (w.screen == main) {
                [w makeKeyWindow];
            }
        }
        if (!still) {
            [NSAnimationContext runAnimationGroup:^(NSAnimationContext *ctx) {
                ctx.duration = 0.35;
                for (NSWindow *w in c.windows) {
                    w.animator.alphaValue = 1;
                }
            }];
        }
    });
}

// ---------------------------------------------------------------------------
// Start hotkey and prompt. ⌥⌘F is a Carbon hot key: it needs no
// Accessibility permission and never sees other keys.

static EventHotKeyRef hotkey;
static EventHandlerRef hotkeyHandler;

static OSStatus hotkeyPressed(EventHandlerCallRef h, EventRef e, void *data) {
    (void)h;
    (void)e;
    (void)data;
    goMenu((char *)"hotkey");
    return noErr;
}

void bar_set_hotkey(int on) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (on && hotkey == NULL) {
            if (hotkeyHandler == NULL) {
                EventTypeSpec t = {kEventClassKeyboard, kEventHotKeyPressed};
                InstallApplicationEventHandler(&hotkeyPressed, 1, &t, NULL, &hotkeyHandler);
            }
            EventHotKeyID hid = {'tock', 1};
            if (RegisterEventHotKey(kVK_ANSI_F, cmdKey | optionKey, hid, GetApplicationEventTarget(), 0,
                                    &hotkey) != noErr) {
                hotkey = NULL;
            }
        } else if (!on && hotkey != NULL) {
            UnregisterEventHotKey(hotkey);
            hotkey = NULL;
        }
    });
}

static BOOL prompting;

// bar_prompt shows the start prompt: "start\ttitle\tdetail\t25,50,90" asks
// for a goal and a length, "stop\ttitle\tdetail" offers to stop. The answer
// goes back as a menu click: "focus:<min>:<goal>" or "stop".
void bar_prompt(const char *spec) {
    NSString *sp = str(spec);
    dispatch_async(dispatch_get_main_queue(), ^{
        if (prompting) {
            return;
        }
        NSArray<NSString *> *f = [sp componentsSeparatedByString:@"\t"];
        if (f.count < 3) {
            return;
        }
        prompting = YES;
        [NSApp activateIgnoringOtherApps:YES];
        NSAlert *a = [NSAlert new];
        a.messageText = f[1];
        a.informativeText = f[2];
        if ([f[0] isEqualToString:@"stop"]) {
            [a addButtonWithTitle:@"Stop session"];
            [a addButtonWithTitle:@"Keep going"];
            if ([a runModal] == NSAlertFirstButtonReturn) {
                goMenu((char *)"stop");
            }
            prompting = NO;
            return;
        }
        NSArray<NSString *> *mins = f.count > 3 ? [f[3] componentsSeparatedByString:@","] : @[ @"25" ];
        NSView *v = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 300, 58)];
        NSTextField *goal = [[NSTextField alloc] initWithFrame:NSMakeRect(0, 32, 300, 24)];
        goal.placeholderString = @"TD 3, exercises 4 to 7";
        NSPopUpButton *len = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(0, 0, 120, 26) pullsDown:NO];
        for (NSString *m in mins) {
            [len addItemWithTitle:[m stringByAppendingString:@" min"]];
            len.lastItem.representedObject = m;
        }
        [v addSubview:goal];
        [v addSubview:len];
        a.accessoryView = v;
        [a addButtonWithTitle:@"Start"];
        [a addButtonWithTitle:@"Cancel"];
        [a layout];
        a.window.initialFirstResponder = goal;
        if ([a runModal] == NSAlertFirstButtonReturn) {
            NSString *g = [[goal.stringValue componentsSeparatedByCharactersInSet:
                                                 [NSCharacterSet newlineCharacterSet]]
                componentsJoinedByString:@" "];
            NSString *id = [NSString stringWithFormat:@"focus:%@:%@", len.selectedItem.representedObject, g];
            goMenu((char *)[id UTF8String]);
        }
        prompting = NO;
    });
}
