// dash_darwin.m: tock's dashboard window. One NSWindow with a WKWebView on
// the page tock web serves on the loopback interface. While it is open the
// app is a regular one, with a Dock icon, a menu bar and a place in ⌘-Tab;
// closing it hands back to the menu bar only.
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include "dash_darwin.h"

@interface DashWindow : NSObject <NSWindowDelegate, WKNavigationDelegate, WKUIDelegate, WKDownloadDelegate>
@property(strong) NSWindow *win;
@property(strong) WKWebView *web;
@property(copy) NSString *base; // scheme://host:port of the page loaded
// Each download's file in progress and where it goes when done; NSNull
// once the user cancelled the sheet and the page has been told.
@property(strong) NSMapTable<WKDownload *, id> *saves;
@end

static DashWindow *dash;
static NSMenu *appMenu, *barMenu; // the regular app's menu; whatever was there

// jsString quotes s as a JavaScript string literal.
static NSString *jsString(NSString *s) {
    NSData *d = [NSJSONSerialization dataWithJSONObject:@[ s ?: @"" ] options:0 error:nil];
    NSString *arr = [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding];
    return [arr substringWithRange:NSMakeRange(1, arr.length - 2)];
}

static NSString *origin(NSURL *u) {
    return [NSString stringWithFormat:@"%@://%@:%@", u.scheme, u.host, u.port];
}

static BOOL local(NSURL *u) {
    return [u.host isEqualToString:@"127.0.0.1"] || [u.host isEqualToString:@"localhost"] || [u.host isEqualToString:@"::1"];
}

// buildMenu is the minimal menu a regular app needs: Quit, Close, the edit
// keys for the label search, Reload.
static NSMenu *buildMenu(void) {
    NSMenu *m = [NSMenu new];
    NSMenuItem *appItem = [m addItemWithTitle:@"tock" action:nil keyEquivalent:@""];
    NSMenu *app = [[NSMenu alloc] initWithTitle:@"tock"];
    [app addItemWithTitle:@"Close Window" action:@selector(performClose:) keyEquivalent:@"w"];
    [app addItem:[NSMenuItem separatorItem]];
    [app addItemWithTitle:@"Hide tock" action:@selector(hide:) keyEquivalent:@"h"];
    [app addItemWithTitle:@"Quit tock" action:@selector(terminate:) keyEquivalent:@"q"];
    appItem.submenu = app;

    NSMenuItem *editItem = [m addItemWithTitle:@"Edit" action:nil keyEquivalent:@""];
    NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
    [edit addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
    [edit addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"Z"];
    [edit addItem:[NSMenuItem separatorItem]];
    [edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
    [edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
    [edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
    [edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
    editItem.submenu = edit;

    NSMenuItem *viewItem = [m addItemWithTitle:@"View" action:nil keyEquivalent:@""];
    NSMenu *view = [[NSMenu alloc] initWithTitle:@"View"];
    [view addItemWithTitle:@"Reload" action:@selector(reload:) keyEquivalent:@"r"];
    [view addItemWithTitle:@"Enter Full Screen" action:@selector(toggleFullScreen:) keyEquivalent:@"f"].keyEquivalentModifierMask =
        NSEventModifierFlagCommand | NSEventModifierFlagControl;
    viewItem.submenu = view;
    return m;
}

@implementation DashWindow

- (void)make {
    NSUInteger style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable |
                       NSWindowStyleMaskResizable | NSWindowStyleMaskFullSizeContentView;
    self.win = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 1280, 820)
                                           styleMask:style
                                             backing:NSBackingStoreBuffered
                                               defer:NO];
    self.win.title = @"tock";
    self.win.titlebarAppearsTransparent = YES;
    self.win.titleVisibility = NSWindowTitleHidden;
    self.win.minSize = NSMakeSize(900, 600);
    self.win.releasedWhenClosed = NO;
    self.win.delegate = self;
    self.win.tabbingMode = NSWindowTabbingModeDisallowed;
    self.win.backgroundColor = [NSColor colorWithName:nil dynamicProvider:^NSColor *(NSAppearance *a) {
        BOOL dark = [[a bestMatchFromAppearancesWithNames:@[ NSAppearanceNameAqua, NSAppearanceNameDarkAqua ]]
            isEqualToString:NSAppearanceNameDarkAqua];
        return dark ? [NSColor colorWithSRGBRed:0x0f / 255.0 green:0x10 / 255.0 blue:0x11 / 255.0 alpha:1] : NSColor.whiteColor;
    }];
    [self.win center];
    [self.win setFrameAutosaveName:@"tock.dashboard"];

    WKWebViewConfiguration *c = [WKWebViewConfiguration new];
    c.applicationNameForUserAgent = @"tock-app"; // the page leaves room for the traffic lights
    c.websiteDataStore = [WKWebsiteDataStore nonPersistentDataStore]; // nothing kept between runs
    self.web = [[WKWebView alloc] initWithFrame:self.win.contentView.bounds configuration:c];
    self.web.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    self.web.navigationDelegate = self;
    self.web.UIDelegate = self;
    [self.web setValue:@NO forKey:@"drawsBackground"]; // no white flash before the page paints
    self.win.contentView = self.web;
}

- (void)open:(NSString *)link {
    NSURL *u = [NSURL URLWithString:link];
    if (!u) return;
    if (!self.win) [self make];
    if (self.web.URL && [origin(u) isEqualToString:self.base]) {
        // Same server: switch the view in place. The page reads ?v= only on
        // load, so pass it as the fragment route it listens to.
        NSString *view = @"today";
        for (NSURLQueryItem *q in [NSURLComponents componentsWithURL:u resolvingAgainstBaseURL:NO].queryItems)
            if ([q.name isEqualToString:@"v"]) view = q.value;
        [self.web evaluateJavaScript:[NSString stringWithFormat:@"location.hash=%@", jsString(view)] completionHandler:nil];
    } else {
        self.base = origin(u);
        [self.web loadRequest:[NSURLRequest requestWithURL:u]];
    }
    if (NSApp.activationPolicy != NSApplicationActivationPolicyRegular) {
        barMenu = NSApp.mainMenu;
        if (!appMenu) appMenu = buildMenu();
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        NSApp.mainMenu = appMenu;
    }
    [self.win makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
}

- (void)reload:(id)sender {
    [self.web reload];
}

- (void)windowWillClose:(NSNotification *)n {
    NSApp.mainMenu = barMenu;
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

// Links off the loopback page open in the default browser.
- (void)webView:(WKWebView *)w
    decidePolicyForNavigationAction:(WKNavigationAction *)a
                    decisionHandler:(void (^)(WKNavigationActionPolicy))h {
    NSURL *u = a.request.URL;
    // Off-page first: a download link to another host still goes to the
    // browser, never fetched from inside this window.
    if (u.host && !local(u)) {
        [[NSWorkspace sharedWorkspace] openURL:u];
        h(WKNavigationActionPolicyCancel);
        return;
    }
    if (a.shouldPerformDownload) { // the page's Export: a blob link with download set
        h(WKNavigationActionPolicyDownload);
        return;
    }
    h(WKNavigationActionPolicyAllow);
}

- (void)webView:(WKWebView *)w navigationAction:(WKNavigationAction *)a didBecomeDownload:(WKDownload *)d {
    d.delegate = self;
}

- (void)webView:(WKWebView *)w navigationResponse:(WKNavigationResponse *)r didBecomeDownload:(WKDownload *)d {
    d.delegate = self;
}

// tell hands the page how a download ended, as a tock-download event.
- (void)tell:(BOOL)ok name:(NSString *)name error:(NSString *)err {
    NSMutableDictionary *r = [@{@"ok" : @(ok)} mutableCopy];
    if (name) r[@"name"] = name;
    if (err) r[@"error"] = err;
    NSData *d = [NSJSONSerialization dataWithJSONObject:r options:0 error:nil];
    NSString *js = [NSString stringWithFormat:@"dispatchEvent(new CustomEvent('tock-download',{detail:%@}))",
                                              [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding]];
    [self.web evaluateJavaScript:js completionHandler:nil];
}

// A download asks where to go with a save sheet on the window, in Downloads
// under the server's name. WebKit writes to a private temporary file; the
// chosen path is only touched once that file is complete, so a failed
// download leaves an existing file as it was.
- (void)download:(WKDownload *)d
    decideDestinationUsingResponse:(NSURLResponse *)r
                 suggestedFilename:(NSString *)name
                 completionHandler:(void (^)(NSURL *))done {
    if (!self.saves) self.saves = [NSMapTable strongToStrongObjectsMapTable];
    NSString *base = name.lastPathComponent;
    NSSavePanel *p = [NSSavePanel savePanel];
    p.nameFieldStringValue = base.length && ![base isEqualToString:@"/"] ? base : @"tock-export";
    p.directoryURL = [NSFileManager.defaultManager URLsForDirectory:NSDownloadsDirectory inDomains:NSUserDomainMask].firstObject;
    [p beginSheetModalForWindow:self.win completionHandler:^(NSModalResponse res) {
        if (res != NSModalResponseOK || !p.URL) {
            [self.saves setObject:NSNull.null forKey:d];
            [self tell:NO name:nil error:nil];
            done(nil);
            return;
        }
        NSFileManager *fm = NSFileManager.defaultManager;
        NSURL *dir = [[NSURL fileURLWithPath:NSTemporaryDirectory()]
            URLByAppendingPathComponent:[@"tock-download-" stringByAppendingString:NSUUID.UUID.UUIDString]];
        NSError *err;
        if (![fm createDirectoryAtURL:dir withIntermediateDirectories:YES attributes:nil error:&err]) {
            [self.saves setObject:NSNull.null forKey:d];
            [self tell:NO name:p.URL.lastPathComponent error:err.localizedDescription];
            done(nil);
            return;
        }
        NSURL *tmp = [dir URLByAppendingPathComponent:p.URL.lastPathComponent];
        [self.saves setObject:@[ tmp, p.URL ] forKey:d];
        done(tmp);
    }];
}

- (void)downloadDidFinish:(WKDownload *)d {
    NSArray<NSURL *> *s = [self.saves objectForKey:d];
    [self.saves removeObjectForKey:d];
    if (![s isKindOfClass:NSArray.class]) return;
    NSURL *tmp = s[0], *dest = s[1];
    NSFileManager *fm = NSFileManager.defaultManager;
    NSError *err;
    BOOL ok;
    if ([fm fileExistsAtPath:dest.path]) {
        // The panel already asked before replacing; swap in one step.
        ok = [fm replaceItemAtURL:dest withItemAtURL:tmp backupItemName:nil options:0 resultingItemURL:nil error:&err];
    } else {
        ok = [fm moveItemAtURL:tmp toURL:dest error:&err];
    }
    [fm removeItemAtURL:tmp.URLByDeletingLastPathComponent error:nil];
    [self tell:ok name:dest.lastPathComponent error:ok ? nil : err.localizedDescription];
}

- (void)download:(WKDownload *)d didFailWithError:(NSError *)error resumeData:(NSData *)resume {
    id s = [self.saves objectForKey:d];
    [self.saves removeObjectForKey:d];
    if (s == NSNull.null) return; // cancelled in the sheet, already told
    NSString *name = nil;
    if ([s isKindOfClass:NSArray.class]) {
        NSArray<NSURL *> *a = s;
        name = a[1].lastPathComponent;
        [NSFileManager.defaultManager removeItemAtURL:a[0].URLByDeletingLastPathComponent error:nil];
    }
    [self tell:NO name:name error:error.localizedDescription ?: @"download failed"];
}

// target=_blank: same rule, never a second web view.
- (WKWebView *)webView:(WKWebView *)w
    createWebViewWithConfiguration:(WKWebViewConfiguration *)c
               forNavigationAction:(WKNavigationAction *)a
                    windowFeatures:(WKWindowFeatures *)f {
    if (a.request.URL) [[NSWorkspace sharedWorkspace] openURL:a.request.URL];
    return nil;
}

@end

void bar_open_dashboard(const char *link) {
    NSString *l = [NSString stringWithUTF8String:link];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!dash) dash = [DashWindow new];
        [dash open:l];
    });
}
