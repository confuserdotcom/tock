// The Cocoa side of the tock menu bar app. Every UI call hops to the main
// thread, which runs [NSApp run]; clicks go back to Go through goMenu and
// goResponse, which only queue them.
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

// spec: one line per item, "id\ttitle"; an empty id is a disabled label and
// a line "-" a separator.
void bar_set_menu(const char *spec) {
    NSString *s = str(spec);
    dispatch_async(dispatch_get_main_queue(), ^{
        NSMenu *m = [NSMenu new];
        m.autoenablesItems = NO;
        for (NSString *line in [s componentsSeparatedByString:@"\n"]) {
            if ([line isEqualToString:@"-"]) {
                [m addItem:[NSMenuItem separatorItem]];
                continue;
            }
            NSArray *f = [line componentsSeparatedByString:@"\t"];
            if (f.count != 2) {
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
            [m addItem:it];
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
