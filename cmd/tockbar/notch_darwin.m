// The notch panel: tock's island, drawn over the notch of the Mac (or as a
// pill under the menu bar without one). Go decides what it shows
// (bar.Island, as JSON); this side only lays it out, moves it and sends
// clicks back through goMenu.
//
// Motion follows the approved prototype: every size is a spring with a
// response (seconds) and a damping ratio, stepped by hand so that a new
// target always starts from where the shape is and how fast it moves.
#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import <objc/runtime.h>

#include "_cgo_export.h"
#include "notch_darwin.h"

// ---------- springs ----------

typedef struct {
    double x, v, t;
    double response, damping;
} Spring;

typedef struct {
    double response, damping;
} SpringParams;

static const SpringParams kEvent = {0.46, 0.78}; // tock opens a moment: a little bounce
static const SpringParams kUser = {0.32, 1.0};   // you hover: none
static const SpringParams kClose = {0.36, 1.0};  // back into the notch
static const SpringParams kFade = {0.22, 1.0};
static const SpringParams kFadeOut = {0.12, 1.0};
static const SpringParams kSlide = {0.42, 0.9};

static void springTo(Spring *s, double t, SpringParams p) {
    s->t = t;
    s->response = p.response;
    s->damping = p.damping;
}

static void springJump(Spring *s, double t) {
    s->x = s->t = t;
    s->v = 0;
}

static void springStep(Spring *s, double dt) {
    if (s->response <= 0) {
        s->response = kUser.response, s->damping = kUser.damping;
    }
    double k = pow(2 * M_PI / s->response, 2), c = 4 * M_PI * s->damping / s->response;
    s->v += (-k * (s->x - s->t) - c * s->v) * dt;
    s->x += s->v * dt;
    if (fabs(s->v) < 0.02 && fabs(s->x - s->t) < 0.02) {
        s->x = s->t;
        s->v = 0;
    }
}

static BOOL springRest(const Spring *s) { return s->v == 0 && s->x == s->t; }

// ---------- colours and type ----------

static NSColor *hexc(unsigned v, double a) {
    return [NSColor colorWithSRGBRed:((v >> 16) & 0xff) / 255.0 green:((v >> 8) & 0xff) / 255.0 blue:(v & 0xff) / 255.0 alpha:a];
}

static NSColor *white(double a) { return [NSColor colorWithWhite:1 alpha:a]; }

// The dark set of DESIGN.md's kind colours: the island is always black.
static NSColor *kindColor(NSString *k) {
    static NSDictionary<NSString *, NSColor *> *m;
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        m = @{
            @"deep" : hexc(0x5fa3c4, 1),
            @"shallow" : hexc(0x8fa1ba, 1),
            @"browse" : hexc(0xb39cc6, 1),
            @"neutral" : hexc(0x6b7079, 1),
            @"distract" : hexc(0xe0726f, 1),
            @"none" : hexc(0x3a3d42, 1),
            @"good" : hexc(0x5cc38a, 1),
            @"break" : hexc(0xe3a446, 1),
        };
    });
    NSColor *c = m[k];
    return c ? c : m[@"none"];
}

static NSFont *sys(CGFloat size, NSFontWeight w) { return [NSFont systemFontOfSize:size weight:w]; }
static NSFont *digits(CGFloat size, NSFontWeight w) { return [NSFont monospacedDigitSystemFontOfSize:size weight:w]; }

static NSString *sval(NSDictionary *d, NSString *k) {
    id v = d[k];
    return [v isKindOfClass:[NSString class]] ? v : @"";
}

static NSDictionary *dval(NSDictionary *d, NSString *k) {
    id v = d[k];
    return [v isKindOfClass:[NSDictionary class]] ? v : nil;
}

static NSArray *aval(NSDictionary *d, NSString *k) {
    id v = d[k];
    return [v isKindOfClass:[NSArray class]] ? v : @[];
}

static long long ival(NSDictionary *d, NSString *k) {
    id v = d[k];
    return [v isKindOfClass:[NSNumber class]] ? [v longLongValue] : 0;
}

static NSTextField *text(NSString *s, NSFont *f, NSColor *c) {
    NSTextField *t = [NSTextField labelWithString:s ? s : @""];
    t.font = f;
    t.textColor = c;
    t.lineBreakMode = NSLineBreakByTruncatingTail;
    t.maximumNumberOfLines = 1;
    [t sizeToFit];
    return t;
}

static NSTextField *wrapped(NSString *s, NSFont *f, NSColor *c, CGFloat width, NSInteger lines) {
    NSTextField *t = [NSTextField wrappingLabelWithString:s ? s : @""];
    t.font = f;
    t.textColor = c;
    t.maximumNumberOfLines = lines;
    t.preferredMaxLayoutWidth = width;
    NSSize sz = [t sizeThatFits:NSMakeSize(width, CGFLOAT_MAX)];
    t.frame = NSMakeRect(0, 0, width, ceil(sz.height));
    return t;
}

static void notchSend(NSString *ident) {
    if (ident.length == 0) {
        return;
    }
    // Plain menu ids do what the menu item does; the runner closes the moment.
    if (![ident hasPrefix:@"notch:"]) {
        ident = [@"notch:menu:" stringByAppendingString:ident];
    }
    goMenu((char *)ident.UTF8String);
}

static NSString *countdown(long long endMS) {
    long long now = (long long)([[NSDate date] timeIntervalSince1970] * 1000);
    long long s = (MAX(endMS - now, 0) + 999) / 1000;
    return [NSString stringWithFormat:@"%lld:%02lld", s / 60, s % 60];
}

// ---------- small views ----------

@interface FlipView : NSView
@end
@implementation FlipView
- (BOOL)isFlipped {
    return YES;
}
@end

// Dot is a kind colour as a small disc.
@interface Dot : NSView
@property(strong) NSColor *color;
@end
@implementation Dot
- (void)drawRect:(NSRect)r {
    (void)r;
    [self.color setFill];
    [[NSBezierPath bezierPathWithOvalInRect:self.bounds] fill];
}
@end

// Pill is a panel button or a label chip: a rounded fill, a title, an
// optional dot and key hint. Clicks send its ident.
@interface Pill : NSControl
@property(copy) NSString *ident;
@property(copy) NSString *title;
@property(copy) NSString *hint;  // "1", "S": shown as a key cap
@property(copy) NSString *extra; // "86%": dimmed after the title
@property(strong) NSColor *dot;
@property BOOL primary, chip, hover;
@end

@implementation Pill
- (BOOL)isFlipped {
    return YES;
}
- (NSDictionary *)attrs {
    NSColor *c = self.primary ? [NSColor blackColor] : white(1);
    return @{NSFontAttributeName : sys(12, NSFontWeightMedium), NSForegroundColorAttributeName : c};
}
- (CGFloat)fitWidth {
    CGFloat w = [self.title sizeWithAttributes:[self attrs]].width + (self.chip ? 22 : 22);
    if (self.dot) {
        w += 13;
    }
    if (self.extra.length) {
        w += [self.extra sizeWithAttributes:@{NSFontAttributeName : digits(11, NSFontWeightRegular)}].width + 6;
    }
    if (self.hint.length) {
        w += 20;
    }
    return ceil(w);
}
- (void)updateTrackingAreas {
    for (NSTrackingArea *a in self.trackingAreas) {
        [self removeTrackingArea:a];
    }
    [self addTrackingArea:[[NSTrackingArea alloc] initWithRect:NSZeroRect
                                                       options:NSTrackingMouseEnteredAndExited | NSTrackingActiveAlways | NSTrackingInVisibleRect
                                                         owner:self
                                                      userInfo:nil]];
}
- (void)mouseEntered:(NSEvent *)e {
    (void)e;
    self.hover = YES;
    self.needsDisplay = YES;
}
- (void)mouseExited:(NSEvent *)e {
    (void)e;
    self.hover = NO;
    self.needsDisplay = YES;
}
- (void)mouseDown:(NSEvent *)e {
    (void)e;
}
- (void)mouseUp:(NSEvent *)e {
    NSPoint p = [self convertPoint:e.locationInWindow fromView:nil];
    if (NSPointInRect(p, self.bounds)) {
        notchSend(self.ident);
    }
}
- (void)drawRect:(NSRect)r {
    (void)r;
    NSRect b = self.bounds;
    CGFloat rad = self.chip ? b.size.height / 2 : 7;
    NSColor *fill = self.primary ? (self.hover ? white(1) : hexc(0xf2f3f5, 1)) : white(self.hover ? 0.2 : (self.chip ? 0.08 : 0.12));
    [fill setFill];
    NSBezierPath *p = [NSBezierPath bezierPathWithRoundedRect:NSInsetRect(b, 0.5, 0.5) xRadius:rad yRadius:rad];
    [p fill];
    if (self.chip && !self.primary) {
        [white(0.14) setStroke];
        [p stroke];
    }
    CGFloat x = 11;
    if (self.dot) {
        [self.dot setFill];
        [[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(x - 1, b.size.height / 2 - 3.5, 7, 7)] fill];
        x += 13;
    }
    NSDictionary *a = [self attrs];
    NSSize ts = [self.title sizeWithAttributes:a];
    [self.title drawAtPoint:NSMakePoint(x, (b.size.height - ts.height) / 2) withAttributes:a];
    x += ts.width + 6;
    if (self.extra.length) {
        NSDictionary *ea = @{NSFontAttributeName : digits(11, NSFontWeightRegular),
                             NSForegroundColorAttributeName : self.primary ? [NSColor colorWithWhite:0 alpha:0.55] : white(0.55)};
        NSSize es = [self.extra sizeWithAttributes:ea];
        [self.extra drawAtPoint:NSMakePoint(x, (b.size.height - es.height) / 2) withAttributes:ea];
        x += es.width + 6;
    }
    if (self.hint.length) {
        NSDictionary *ha = @{NSFontAttributeName : sys(10, NSFontWeightMedium),
                             NSForegroundColorAttributeName : self.primary ? [NSColor colorWithWhite:0 alpha:0.5] : white(0.5)};
        NSSize hs = [self.hint sizeWithAttributes:ha];
        NSRect cap = NSMakeRect(x, (b.size.height - 15) / 2, MAX(15, hs.width + 8), 15);
        [(self.primary ? [NSColor colorWithWhite:0 alpha:0.25] : white(0.22)) setStroke];
        [[NSBezierPath bezierPathWithRoundedRect:NSInsetRect(cap, 0.5, 0.5) xRadius:4 yRadius:4] stroke];
        [self.hint drawAtPoint:NSMakePoint(NSMidX(cap) - hs.width / 2, NSMidY(cap) - hs.height / 2) withAttributes:ha];
    }
}
@end

// Cells is a number row: one bordered strip split by hairlines.
@interface Cells : FlipView
@property(strong) NSArray<NSDictionary *> *cells;
@end
@implementation Cells
- (void)drawRect:(NSRect)r {
    (void)r;
    NSRect b = NSInsetRect(self.bounds, 0.5, 0.5);
    [white(0.12) setStroke];
    [[NSBezierPath bezierPathWithRoundedRect:b xRadius:8 yRadius:8] stroke];
    NSUInteger n = MAX(self.cells.count, 1u);
    CGFloat w = self.bounds.size.width / n;
    for (NSUInteger i = 0; i < self.cells.count; i++) {
        NSDictionary *c = self.cells[i];
        CGFloat x = i * w;
        if (i > 0) {
            [white(0.12) setStroke]; // drawing text changes the current colour
            NSBezierPath *l = [NSBezierPath bezierPath];
            [l moveToPoint:NSMakePoint(floor(x) + 0.5, 0)];
            [l lineToPoint:NSMakePoint(floor(x) + 0.5, self.bounds.size.height)];
            [l stroke];
        }
        [sval(c, @"k") drawAtPoint:NSMakePoint(x + 10, 6)
                    withAttributes:@{NSFontAttributeName : sys(10.5, NSFontWeightRegular), NSForegroundColorAttributeName : white(0.55)}];
        [sval(c, @"v") drawAtPoint:NSMakePoint(x + 10, 19)
                    withAttributes:@{NSFontAttributeName : digits(15, NSFontWeightSemibold), NSForegroundColorAttributeName : white(1)}];
        NSString *tone = sval(c, @"tone");
        NSColor *tc = [tone isEqualToString:@"good"] ? kindColor(@"good") : [tone isEqualToString:@"bad"] ? kindColor(@"distract") : white(0.55);
        [sval(c, @"c") drawAtPoint:NSMakePoint(x + 10, 39)
                    withAttributes:@{NSFontAttributeName : digits(10.5, NSFontWeightRegular), NSForegroundColorAttributeName : tc}];
    }
}
@end

// Strip is time as kind-coloured blocks with hour labels under it.
@interface Strip : FlipView
@property(strong) NSArray<NSString *> *kinds;
@property(strong) NSArray<NSString *> *axis;
@end
@implementation Strip
- (void)drawRect:(NSRect)r {
    (void)r;
    CGFloat w = self.bounds.size.width;
    NSUInteger n = MAX(self.kinds.count, 1u);
    CGFloat cw = w / n;
    NSBezierPath *clip = [NSBezierPath bezierPathWithRoundedRect:NSMakeRect(0, 0, w, 8) xRadius:2 yRadius:2];
    [NSGraphicsContext saveGraphicsState];
    [clip addClip];
    for (NSUInteger i = 0; i < self.kinds.count; i++) {
        [kindColor(self.kinds[i]) setFill];
        NSRectFill(NSMakeRect(floor(i * cw), 0, ceil(cw) - (i + 1 < n ? 1 : 0), 8));
    }
    [NSGraphicsContext restoreGraphicsState];
    NSDictionary *a = @{NSFontAttributeName : digits(10, NSFontWeightRegular), NSForegroundColorAttributeName : white(0.45)};
    NSUInteger m = self.axis.count;
    for (NSUInteger i = 0; i < m; i++) {
        NSString *s = self.axis[i];
        NSSize sz = [s sizeWithAttributes:a];
        CGFloat x = m == 1 ? 0 : (w - sz.width) * i / (m - 1);
        [s drawAtPoint:NSMakePoint(x, 12) withAttributes:a];
    }
}
@end

// Rows is a short table: name left, time right, hairlines between.
@interface Rows : FlipView
@property(strong) NSArray<NSArray *> *rows;
@end
@implementation Rows
- (void)drawRect:(NSRect)r {
    (void)r;
    CGFloat w = self.bounds.size.width;
    NSDictionary *na = @{NSFontAttributeName : sys(12, NSFontWeightRegular), NSForegroundColorAttributeName : white(0.92)};
    NSDictionary *va = @{NSFontAttributeName : digits(12, NSFontWeightRegular), NSForegroundColorAttributeName : white(0.75)};
    for (NSUInteger i = 0; i < self.rows.count; i++) {
        NSArray *row = self.rows[i];
        if (row.count < 2) {
            continue;
        }
        CGFloat y = i * 21;
        if (i > 0) {
            [white(0.08) setFill];
            NSRectFill(NSMakeRect(0, y, w, 1));
        }
        [row[0] drawAtPoint:NSMakePoint(0, y + 3) withAttributes:na];
        NSSize vs = [row[1] sizeWithAttributes:va];
        [row[1] drawAtPoint:NSMakePoint(w - vs.width, y + 3) withAttributes:va];
    }
}
@end

// ---------- the island ----------

@class NotchPanel;

@interface Island : FlipView
@property(strong) NotchPanel *panel;
@property(strong) NSDictionary *island; // what Go asked for
@property(strong) NSView *content;      // the content shown, fading in
@property(strong) NSMutableArray<NSView *> *leaving;
@property(strong) NSMutableArray<NSTextField *> *timers; // countdown labels
@property(strong) NSView *track;                         // labelling cards
@property NSInteger trackAt;                             // the card the track shows
@property BOOL panelShown;                               // the panel, not the face
@property BOOL hoverOpen;                                // the panel is a hover
@property CGSize target;
@property CGFloat targetR;
@property NSSize contentSize;
@end

@interface NotchPanel : NSPanel
@end
@implementation NotchPanel
- (BOOL)canBecomeKeyWindow {
    return YES;
}
- (BOOL)canBecomeMainWindow {
    return NO;
}
// The panel sits on the menu bar and over the notch: AppKit would push it
// below them.
- (NSRect)constrainFrameRect:(NSRect)r toScreen:(NSScreen *)s {
    (void)s;
    return r;
}
@end

static Island *island;
static Spring W, H, R, O, CO, CY, TX; // width, height, radius, opacity, content opacity, content y, track x
static NSTimer *frameTimer, *clockTimer, *hoverTimer, *leaveTimer, *autoTimer, *slideTimer;
static NSScreen *screen;
static CGFloat notchW, notchH, notchMidX, screenTop;
static BOOL hasNotch, reduceMotion, hidden = YES;
static double leavingAlpha;
static id clickMonitor;
static double wheelAcc;
static CFTimeInterval lastFrame;

static const CGFloat kEar = 8;    // the fillets that join the menu bar
static const CGFloat kPad = 18;   // panel side padding
static const CGFloat kGap = 6;    // between the face band and the body
static const CGFloat kMaxW = 720; // the window never needs more

static void kick(void);
static void layoutWindow(BOOL settle);

// pickScreen finds the screen with a notch, else the main one, and its
// notch: width from the menu bar areas either side of it, height from the
// safe area.
static void pickScreen(void) {
    screen = [NSScreen mainScreen];
    hasNotch = NO;
    if (@available(macOS 12.0, *)) {
        for (NSScreen *s in [NSScreen screens]) {
            if (s.safeAreaInsets.top > 0) {
                screen = s;
                hasNotch = YES;
                break;
            }
        }
    }
    NSRect f = screen.frame;
    screenTop = NSMaxY(f);
    notchMidX = NSMidX(f);
    notchH = [[NSStatusBar systemStatusBar] thickness];
    notchW = 0;
    if (hasNotch) {
        if (@available(macOS 12.0, *)) {
            NSRect l = screen.auxiliaryTopLeftArea, r = screen.auxiliaryTopRightArea;
            notchW = f.size.width - l.size.width - r.size.width;
            notchMidX = NSMaxX(l) + notchW / 2;
            notchH = screen.safeAreaInsets.top;
        }
        if (notchW <= 0) {
            hasNotch = NO;
            notchW = 0;
        }
    }
}

// The shape at rest for the island: its face, or its panel when open.
static CGFloat faceHeight(void) { return hasNotch ? notchH : 32; }

@implementation Island

- (instancetype)initWithFrame:(NSRect)f {
    if ((self = [super initWithFrame:f])) {
        _leaving = [NSMutableArray new];
        _timers = [NSMutableArray new];
        self.wantsLayer = YES;
    }
    return self;
}

- (BOOL)acceptsFirstResponder {
    return YES;
}

- (BOOL)acceptsFirstMouse:(NSEvent *)e {
    (void)e;
    return YES;
}

// shapeRect is the island in view coordinates: centred, from the top.
- (NSRect)shapeRect {
    CGFloat w = MAX(W.x, 0), h = MAX(H.x, 0);
    CGFloat top = hasNotch ? 0 : (self.bounds.size.height - h) / 2;
    if (hasNotch) {
        top = 0;
    }
    return NSMakeRect((self.bounds.size.width - w) / 2, top, w, h);
}

- (void)drawRect:(NSRect)dirty {
    (void)dirty;
    NSRect s = [self shapeRect];
    if (s.size.width < 1 || s.size.height < 0.5) {
        return;
    }
    CGFloat r = MIN(MAX(R.x, 0), MIN(s.size.width, s.size.height * (hasNotch ? 1 : 0.5)));
    [[NSColor colorWithWhite:0 alpha:MIN(MAX(O.x, 0), 1)] setFill];
    NSBezierPath *p = [NSBezierPath bezierPath];
    CGFloat x0 = NSMinX(s), x1 = NSMaxX(s), y1 = NSMaxY(s);
    if (hasNotch) {
        // flat top that runs into the menu bar through two concave fillets
        CGFloat e = kEar, k = 0.45;
        [p moveToPoint:NSMakePoint(x0 - e, 0)];
        [p lineToPoint:NSMakePoint(x1 + e, 0)];
        [p curveToPoint:NSMakePoint(x1, MIN(e, y1)) controlPoint1:NSMakePoint(x1 + e * k, 0) controlPoint2:NSMakePoint(x1, MIN(e, y1) * (1 - k))];
        [p lineToPoint:NSMakePoint(x1, y1 - r)];
        [p curveToPoint:NSMakePoint(x1 - r, y1) controlPoint1:NSMakePoint(x1, y1 - r * 0.45) controlPoint2:NSMakePoint(x1 - r * 0.45, y1)];
        [p lineToPoint:NSMakePoint(x0 + r, y1)];
        [p curveToPoint:NSMakePoint(x0, y1 - r) controlPoint1:NSMakePoint(x0 + r * 0.45, y1) controlPoint2:NSMakePoint(x0, y1 - r * 0.45)];
        [p lineToPoint:NSMakePoint(x0, MIN(e, y1))];
        [p curveToPoint:NSMakePoint(x0 - e, 0) controlPoint1:NSMakePoint(x0, MIN(e, y1) * (1 - k)) controlPoint2:NSMakePoint(x0 - e * k, 0)];
        [p closePath];
    } else {
        p = [NSBezierPath bezierPathWithRoundedRect:s xRadius:r yRadius:r];
    }
    [p fill];
}

// ----- hover, keys, wheel -----

- (void)updateTrackingAreas {
    for (NSTrackingArea *a in self.trackingAreas) {
        [self removeTrackingArea:a];
    }
    [self addTrackingArea:[[NSTrackingArea alloc] initWithRect:NSZeroRect
                                                       options:NSTrackingMouseEnteredAndExited | NSTrackingActiveAlways | NSTrackingInVisibleRect
                                                         owner:self
                                                      userInfo:nil]];
}

- (BOOL)isOpenMoment {
    return [self.island[@"open"] boolValue];
}

- (void)mouseEntered:(NSEvent *)e {
    (void)e;
    [leaveTimer invalidate];
    leaveTimer = nil;
    if (self.isOpenMoment || self.panelShown || !dval(self.island, @"hover")) {
        return;
    }
    [hoverTimer invalidate];
    hoverTimer = [NSTimer scheduledTimerWithTimeInterval:0.11
                                                 repeats:NO
                                                   block:^(NSTimer *t) {
                                                       (void)t;
                                                       hoverTimer = nil;
                                                       self.hoverOpen = YES;
                                                       [self showPanel:YES params:kUser];
                                                   }];
}

- (void)mouseExited:(NSEvent *)e {
    (void)e;
    [hoverTimer invalidate];
    hoverTimer = nil;
    if (!self.hoverOpen) {
        return;
    }
    [leaveTimer invalidate];
    leaveTimer = [NSTimer scheduledTimerWithTimeInterval:0.32
                                                 repeats:NO
                                                   block:^(NSTimer *t) {
                                                       (void)t;
                                                       leaveTimer = nil;
                                                       self.hoverOpen = NO;
                                                       [self showPanel:NO params:kClose];
                                                   }];
}

- (void)mouseDown:(NSEvent *)e {
    (void)e;
    [self.window makeFirstResponder:self];
}

// dismiss sends an open moment home; Go draws what comes next.
- (void)dismiss {
    if (self.isOpenMoment) {
        notchSend(@"notch:close");
    } else if (self.hoverOpen) {
        self.hoverOpen = NO;
        [self showPanel:NO params:kClose];
    }
}

- (void)keyDown:(NSEvent *)e {
    NSString *c = e.charactersIgnoringModifiers;
    if (e.keyCode == 53) { // Esc
        [self dismiss];
        return;
    }
    if ([sval(self.island, @"state") isEqualToString:@"label"] && self.panelShown) {
        NSDictionary *p = dval(self.island, @"panel");
        NSInteger at = (NSInteger)ival(p, @"at");
        NSArray *es = aval(p, @"entries");
        if (at < (NSInteger)es.count) {
            NSArray *opts = aval(es[at], @"opts");
            NSInteger pick = -1;
            if (e.keyCode == 36 || e.keyCode == 76) {
                pick = 0;
            } else if (c.length == 1 && [c characterAtIndex:0] >= '1' && [c characterAtIndex:0] <= '3') {
                pick = [c characterAtIndex:0] - '1';
            } else if ([c.lowercaseString isEqualToString:@"s"]) {
                notchSend([NSString stringWithFormat:@"notch:skip:%ld", (long)at]);
                return;
            }
            if (pick >= 0 && pick < (NSInteger)opts.count) {
                notchSend([NSString stringWithFormat:@"notch:pick:%ld:%ld", (long)at, (long)pick]);
                return;
            }
        }
    }
    [super keyDown:e];
}

// A swipe up (or down) on an open panel sends it home, carrying the
// gesture's speed into the height spring.
- (void)scrollWheel:(NSEvent *)e {
    if (!self.panelShown) {
        return;
    }
    if (e.phase == NSEventPhaseBegan) {
        wheelAcc = 0;
    }
    wheelAcc += e.scrollingDeltaY;
    if (fabs(wheelAcc) > 40) {
        wheelAcc = 0;
        H.v = -MIN(fabs(e.scrollingDeltaY) * 30, 1400);
        [self dismiss];
    }
}

// ----- content -----

- (NSView *)wingsWidth:(CGFloat)w face:(NSDictionary *)f {
    FlipView *v = [[FlipView alloc] initWithFrame:NSMakeRect(0, 0, w, faceHeight())];
    CGFloat h = faceHeight();
    CGFloat x = 14;
    NSString *dot = sval(f, @"dot");
    if (dot.length) {
        Dot *d = [[Dot alloc] initWithFrame:NSMakeRect(x, h / 2 - 3.5, 7, 7)];
        d.color = kindColor(dot);
        [v addSubview:d];
        x += 14;
    }
    NSFont *ft = sys(13, NSFontWeightMedium);
    NSString *left = sval(f, @"left");
    BOOL warm = [dot isEqualToString:@"break"];
    NSColor *ink = warm ? hexc(0xf1e4cf, 1) : white(0.95);
    if (left.length) {
        NSTextField *t = text(left, ft, ink);
        t.frame = NSMakeRect(x, (h - t.frame.size.height) / 2, MIN(t.frame.size.width, w / 2 - x - notchW / 2 - 4), t.frame.size.height);
        [v addSubview:t];
        x += t.frame.size.width + 6;
    }
    NSString *ld = sval(f, @"left_dim");
    if (ld.length) {
        NSTextField *t = text(ld, sys(13, NSFontWeightRegular), white(0.55));
        t.frame = NSMakeRect(x, (h - t.frame.size.height) / 2, t.frame.size.width, t.frame.size.height);
        [v addSubview:t];
    }
    CGFloat rx = w - 14;
    long long end = ival(f, @"end_ms");
    NSString *right = end > 0 ? countdown(end) : sval(f, @"right");
    NSString *rd = sval(f, @"right_dim");
    NSTextField *rt = nil;
    if (right.length) {
        rt = text(right, digits(13, NSFontWeightMedium), ink);
        if (end > 0) {
            rt.tag = 1;
            rt.alignment = NSTextAlignmentRight;
            objc_setAssociatedObject(rt, "end", @(end), OBJC_ASSOCIATION_RETAIN);
            rt.frame = NSMakeRect(0, 0, MAX(rt.frame.size.width, 40), rt.frame.size.height);
            [self.timers addObject:rt];
        }
    }
    // the dim text sits after the value: "18:42 left", "in 2m" puts it before
    BOOL dimFirst = [rd isEqualToString:@"in"];
    NSTextField *dt = rd.length ? text(rd, sys(13, NSFontWeightRegular), white(0.55)) : nil;
    NSArray *order = dimFirst ? @[ rt ?: [NSNull null], dt ?: [NSNull null] ] : @[ dt ?: [NSNull null], rt ?: [NSNull null] ];
    for (id o in order) {
        if (o == [NSNull null]) {
            continue;
        }
        NSTextField *t = o;
        rx -= t.frame.size.width;
        t.frame = NSMakeRect(rx, (h - t.frame.size.height) / 2, t.frame.size.width, t.frame.size.height);
        [v addSubview:t];
        rx -= 5;
    }
    return v;
}

// faceWidth is the compact island's width: the notch plus a wing either
// side wide enough for the longer side.
- (CGFloat)faceWidth:(NSDictionary *)f {
    CGFloat side = 0;
    NSFont *ft = sys(13, NSFontWeightMedium);
    CGFloat l = 14 + (sval(f, @"dot").length ? 14 : 0) + [sval(f, @"left") sizeWithAttributes:@{NSFontAttributeName : ft}].width;
    if (sval(f, @"left_dim").length) {
        l += 6 + [sval(f, @"left_dim") sizeWithAttributes:@{NSFontAttributeName : ft}].width;
    }
    NSString *right = ival(f, @"end_ms") > 0 ? @"88:88" : sval(f, @"right");
    CGFloat r = 14 + [right sizeWithAttributes:@{NSFontAttributeName : digits(13, NSFontWeightMedium)}].width;
    if (sval(f, @"right_dim").length) {
        r += 5 + [sval(f, @"right_dim") sizeWithAttributes:@{NSFontAttributeName : ft}].width;
    }
    side = MIN(MAX(MAX(l, r) + 12, 64), 200);
    return (hasNotch ? notchW : 16) + 2 * side;
}

- (CGFloat)panelWidth:(NSString *)state {
    NSDictionary *w = @{@"done" : @580, @"recap" : @640, @"label" : @600, @"drift" : @540, @"meet" : @480};
    CGFloat v = [w[state] doubleValue];
    return MAX(v ? v : 440, (hasNotch ? notchW : 0) + 220);
}

// buttonsRow lays pills out from x = kPad, wrapping; returns the height.
- (CGFloat)addButtons:(NSArray *)bs to:(NSView *)v y:(CGFloat)y width:(CGFloat)w {
    CGFloat x = kPad, rowH = 26;
    for (NSDictionary *b in bs) {
        Pill *p = [[Pill alloc] initWithFrame:NSZeroRect];
        p.ident = sval(b, @"id");
        p.title = sval(b, @"title");
        p.primary = [b[@"primary"] boolValue];
        CGFloat pw = [p fitWidth];
        if (x > kPad && x + pw > w - kPad) {
            x = kPad;
            y += rowH + 8;
        }
        p.frame = NSMakeRect(x, y, pw, rowH);
        [v addSubview:p];
        x += pw + 8;
    }
    return bs.count ? rowH : 0;
}

- (NSView *)cardFor:(NSDictionary *)e index:(NSInteger)i width:(CGFloat)w height:(CGFloat *)outH {
    FlipView *c = [[FlipView alloc] initWithFrame:NSMakeRect(0, 0, w, 10)];
    CGFloat y = 0;
    // app badge, title, sub, duration
    NSString *app = sval(e, @"app");
    NSTextField *badge = text(app.length ? [[app substringToIndex:1] uppercaseString] : @"·", sys(13, NSFontWeightBold), white(0.9));
    FlipView *bv = [[FlipView alloc] initWithFrame:NSMakeRect(kPad, y, 28, 28)];
    bv.wantsLayer = YES;
    bv.layer.backgroundColor = white(0.12).CGColor;
    bv.layer.cornerRadius = 7;
    badge.frame = NSMakeRect((28 - badge.frame.size.width) / 2, (28 - badge.frame.size.height) / 2, badge.frame.size.width, badge.frame.size.height);
    [bv addSubview:badge];
    [c addSubview:bv];
    NSTextField *dur = text(sval(e, @"dur"), digits(13, NSFontWeightMedium), white(0.95));
    dur.frame = NSMakeRect(w - kPad - dur.frame.size.width, y + 6, dur.frame.size.width, dur.frame.size.height);
    [c addSubview:dur];
    CGFloat tw = w - kPad * 2 - 38 - dur.frame.size.width - 8;
    NSTextField *title = text(sval(e, @"title"), sys(13.5, NSFontWeightSemibold), white(1));
    title.frame = NSMakeRect(kPad + 38, y - 1, tw, title.frame.size.height);
    [c addSubview:title];
    NSTextField *sub = text(sval(e, @"sub"), sys(11.5, NSFontWeightRegular), white(0.55));
    sub.frame = NSMakeRect(kPad + 38, y + 15, tw, sub.frame.size.height);
    [c addSubview:sub];
    y += 38;
    // the label chips, then Skip
    CGFloat x = kPad;
    NSInteger picked = (NSInteger)ival(e, @"picked");
    BOOL done = sval(e, @"saved").length > 0;
    NSArray *opts = aval(e, @"opts");
    for (NSUInteger j = 0; j <= opts.count; j++) {
        Pill *p = [[Pill alloc] initWithFrame:NSZeroRect];
        p.chip = YES;
        if (j < opts.count) {
            NSDictionary *o = opts[j];
            p.title = sval(o, @"label");
            p.dot = kindColor(sval(o, @"kind"));
            long long pct = ival(o, @"pct");
            p.extra = pct > 0 ? [NSString stringWithFormat:@"%lld%%", pct] : nil;
            p.hint = [NSString stringWithFormat:@"%lu", (unsigned long)j + 1];
            p.ident = done ? @"" : [NSString stringWithFormat:@"notch:pick:%ld:%lu", (long)i, (unsigned long)j];
            p.primary = done && picked == (NSInteger)j;
        } else {
            p.title = @"Skip";
            p.hint = @"S";
            p.ident = done ? @"" : [NSString stringWithFormat:@"notch:skip:%ld", (long)i];
        }
        CGFloat pw = [p fitWidth];
        if (x > kPad && x + pw > w - kPad) {
            x = kPad;
            y += 34;
        }
        p.frame = NSMakeRect(x, y, pw, 26);
        [c addSubview:p];
        x += pw + 8;
    }
    y += 26 + 8;
    NSString *saved = sval(e, @"saved");
    NSTextField *st = wrapped(saved.length ? saved : @" ", sys(12, NSFontWeightRegular),
                              [saved hasPrefix:@"Not saved"] ? kindColor(@"distract") : white(0.78), w - kPad * 2, 2);
    st.frame = NSMakeRect(kPad, y, st.frame.size.width, MAX(st.frame.size.height, 16));
    [c addSubview:st];
    y += st.frame.size.height + 10;
    c.frame = NSMakeRect(0, 0, w, y);
    *outH = y;
    return c;
}

// panelView lays out the open panel for state; returns it with its size.
- (NSView *)panelView:(NSDictionary *)p state:(NSString *)state size:(NSSize *)out {
    CGFloat w = [self panelWidth:state];
    FlipView *v = [[FlipView alloc] initWithFrame:NSMakeRect(0, 0, w, 100)];
    [v addSubview:[self wingsWidth:w face:dval(p, @"face")]];
    CGFloat y = faceHeight() + kGap;
    CGFloat inner = w - kPad * 2;
    NSString *lead = sval(p, @"lead"), *sub = sval(p, @"sub");
    if (lead.length) {
        NSTextField *t = text(lead, sys(14, NSFontWeightSemibold), white(1));
        t.frame = NSMakeRect(kPad, y, inner, t.frame.size.height);
        [v addSubview:t];
        y += t.frame.size.height + 1;
    }
    if (sub.length) {
        NSTextField *t = wrapped(sub, sys(12, NSFontWeightRegular), white(0.62), inner, 3);
        t.frame = NSMakeRect(kPad, y, inner, t.frame.size.height);
        [v addSubview:t];
        y += t.frame.size.height;
    }
    if (lead.length || sub.length) {
        y += 12;
    }
    NSArray *cells = aval(p, @"cells");
    if (cells.count) {
        Cells *c = [[Cells alloc] initWithFrame:NSMakeRect(kPad, y, inner, 56)];
        c.cells = cells;
        [v addSubview:c];
        y += 56 + 12;
    }
    NSArray *strip = aval(p, @"strip"), *rows = aval(p, @"rows");
    if (strip.count || rows.count) {
        CGFloat sw = rows.count ? floor(inner * 0.55) : inner;
        CGFloat bh = 0;
        if (strip.count) {
            Strip *s = [[Strip alloc] initWithFrame:NSMakeRect(kPad, y + 4, sw, 26)];
            s.kinds = strip;
            s.axis = aval(p, @"axis");
            [v addSubview:s];
            bh = 30;
        }
        if (rows.count) {
            CGFloat rx = strip.count ? kPad + sw + 18 : kPad;
            Rows *r = [[Rows alloc] initWithFrame:NSMakeRect(rx, y, inner - (rx - kPad), rows.count * 21)];
            r.rows = rows;
            [v addSubview:r];
            bh = MAX(bh, rows.count * 21);
        }
        y += bh + 12;
    }
    NSArray *entries = aval(p, @"entries");
    NSInteger at = (NSInteger)ival(p, @"at");
    if (entries.count && at < (NSInteger)entries.count) {
        FlipView *clip = [[FlipView alloc] initWithFrame:NSMakeRect(0, y, w, 10)];
        clip.wantsLayer = YES;
        clip.layer.masksToBounds = YES;
        FlipView *track = [[FlipView alloc] initWithFrame:NSZeroRect];
        CGFloat ch = 0;
        for (NSUInteger i = 0; i < entries.count; i++) {
            CGFloat h = 0;
            NSView *card = [self cardFor:entries[i] index:(NSInteger)i width:w height:&h];
            card.frame = NSMakeRect(i * w, 0, w, h);
            [track addSubview:card];
            ch = MAX(ch, h);
        }
        track.frame = NSMakeRect(0, 0, w * entries.count, ch);
        clip.frame = NSMakeRect(0, y, w, ch);
        [clip addSubview:track];
        [v addSubview:clip];
        self.track = track;
        y += ch;
    } else {
        self.track = nil;
    }
    CGFloat bh = [self addButtons:aval(p, @"buttons") to:v y:y width:w];
    y += bh ? bh + 16 : 4;
    v.frame = NSMakeRect(0, 0, w, y);
    *out = NSMakeSize(w, y);
    return v;
}

// show puts v in as the content: the old one fades out, v fades in behind
// the shape, from a little above.
- (void)show:(NSView *)v size:(NSSize)sz fade:(BOOL)fade {
    if (self.content) {
        if (fade) {
            [self.leaving addObject:self.content];
            leavingAlpha = 1;
        } else {
            [self.content removeFromSuperview];
        }
    }
    self.content = v;
    self.contentSize = sz;
    v.alphaValue = fade ? 0 : 1;
    [self addSubview:v];
    if (fade && !reduceMotion) {
        springJump(&CO, 0);
        springJump(&CY, -6);
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.09 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
            if (self.content == v) {
                springTo(&CO, 1, kFade);
                springTo(&CY, 0, kFade);
                kick();
            }
        });
    } else if (fade) { // reduced motion: a plain cross-fade
        springJump(&CY, 0);
        springJump(&CO, 0);
        springTo(&CO, 1, (SpringParams){0.15, 1});
    } else {
        springJump(&CO, 1);
        springJump(&CY, 0);
    }
}

// resize aims the springs at the size of what shows.
- (void)aimWidth:(CGFloat)w height:(CGFloat)h radius:(CGFloat)r params:(SpringParams)p {
    self.target = CGSizeMake(w, h);
    self.targetR = r;
    if (reduceMotion) {
        springJump(&W, w);
        springJump(&H, h);
        springJump(&R, r);
    } else {
        springTo(&W, w, p);
        springTo(&H, h, p);
        springTo(&R, r, p);
    }
    layoutWindow(NO);
    kick();
}

- (void)rebuildFade:(BOOL)fade params:(SpringParams)p {
    [self.timers removeAllObjects];
    NSString *state = sval(self.island, @"state");
    BOOL quiet = [state isEqualToString:@"quiet"];
    if (self.panelShown) {
        NSDictionary *pd = dval(self.island, self.isOpenMoment ? @"panel" : @"hover");
        if (pd) {
            NSSize sz;
            NSView *v = [self panelView:pd state:state size:&sz];
            [self show:v size:sz fade:fade];
            [self aimWidth:sz.width height:sz.height radius:26 params:p];
            [self placeTrack:YES];
            return;
        }
        self.panelShown = NO;
    }
    NSDictionary *f = dval(self.island, @"face");
    if (quiet) {
        [self show:[[FlipView alloc] initWithFrame:NSZeroRect] size:NSZeroSize fade:fade];
        [self aimWidth:hasNotch ? notchW : 160 height:hasNotch ? notchH : 0 radius:hasNotch ? 10 : 16 params:p];
        return;
    }
    CGFloat w = [self faceWidth:f];
    NSView *v = [self wingsWidth:w face:f];
    [self show:v size:NSMakeSize(w, faceHeight()) fade:fade];
    [self aimWidth:w height:faceHeight() radius:hasNotch ? 12 : faceHeight() / 2 params:p];
}

- (void)showPanel:(BOOL)open params:(SpringParams)p {
    self.panelShown = open;
    [self rebuildFade:YES params:p];
}

// placeTrack moves the labelling cards to the shown entry: right away, or
// once the saved line of the entry just picked has been read.
- (void)placeTrack:(BOOL)fresh {
    if (!self.track) {
        return;
    }
    NSDictionary *p = dval(self.island, @"panel");
    NSInteger at = (NSInteger)ival(p, @"at");
    NSArray *es = aval(p, @"entries");
    if (fresh && at > self.trackAt && self.trackAt < (NSInteger)es.count) {
        // stay on the entry just answered, then slide
        springJump(&TX, self.trackAt);
        NSInteger from = self.trackAt;
        BOOL saved = sval(es[from], @"saved").length > 0;
        [slideTimer invalidate];
        slideTimer = [NSTimer scheduledTimerWithTimeInterval:saved ? 1.15 : 0.05
                                                     repeats:NO
                                                       block:^(NSTimer *t) {
                                                           (void)t;
                                                           springTo(&TX, at, kSlide);
                                                           kick();
                                                       }];
    } else {
        springJump(&TX, MIN(at, MAX((NSInteger)es.count - 1, 0)));
    }
    self.trackAt = at;
}

- (void)paint {
    NSRect s = [self shapeRect];
    CGFloat cw = self.contentSize.width, ch = self.contentSize.height;
    NSRect cr = NSMakeRect(NSMidX(s) - cw / 2, NSMinY(s) + CY.x, cw, ch);
    self.content.frame = cr;
    self.content.alphaValue = MIN(MAX(CO.x, 0), 1);
    // clip the content to the shape as it grows
    if (!self.content.layer) {
        self.content.wantsLayer = YES;
    }
    CGFloat vis = MAX(0, NSMaxY(s) - NSMinY(cr));
    CALayer *mask = self.content.layer.mask;
    if (!mask) {
        mask = [CALayer layer];
        mask.backgroundColor = NSColor.blackColor.CGColor;
        self.content.layer.mask = mask;
    }
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    // layer coordinates are not flipped: keep the top part of the content
    CGFloat mx = MAX(0, NSMinX(s) - NSMinX(cr));
    mask.frame = CGRectMake(mx, MAX(0, ch - vis), MIN(cw, s.size.width), MIN(vis, ch));
    if (self.track) {
        self.track.frameOrigin = NSMakePoint(-TX.x * cw, 0);
    }
    [CATransaction commit];
    for (NSView *l in self.leaving) {
        l.alphaValue = leavingAlpha;
    }
    self.needsDisplay = YES;
}

@end

// ---------- window and frame loop ----------

static NotchPanel *panel;

// layoutWindow sizes the window to the island: while it moves, to cover
// both where it is and where it goes; at rest, to the island only, so the
// rest of the menu bar and the screen keep their clicks.
static void layoutWindow(BOOL settle) {
    if (!panel) {
        return;
    }
    CGFloat w = settle ? MAX(W.x, 0) : MAX(MAX(W.x, 0), island.target.width);
    CGFloat h = settle ? MAX(H.x, 0) : MAX(MAX(H.x, 0), island.target.height);
    w = MIN(w, kMaxW) + (hasNotch ? kEar * 2 : 0);
    h = MIN(h + (settle ? 0 : 24), 420);
    CGFloat top = hasNotch ? screenTop : screenTop - [[NSStatusBar systemStatusBar] thickness] - 6;
    if (!hasNotch) {
        h = MAX(h, 1);
    }
    NSRect f = NSMakeRect(round(notchMidX - w / 2), top - h, ceil(w), ceil(h));
    if (!NSEqualRects(f, panel.frame)) {
        [panel setFrame:f display:NO];
    }
    island.frame = NSMakeRect(0, 0, f.size.width, f.size.height);
}

static BOOL allRest(void) {
    Spring *ss[] = {&W, &H, &R, &O, &CO, &CY, &TX};
    for (size_t i = 0; i < sizeof ss / sizeof ss[0]; i++) {
        if (!springRest(ss[i])) {
            return NO;
        }
    }
    return leavingAlpha <= 0;
}

static void frame(void) {
    CFTimeInterval now = CACurrentMediaTime();
    double dt = MIN(0.05, now - lastFrame);
    lastFrame = now;
    Spring *ss[] = {&W, &H, &R, &O, &CO, &CY, &TX};
    for (int k = 0; k < 4; k++) {
        for (size_t i = 0; i < sizeof ss / sizeof ss[0]; i++) {
            springStep(ss[i], dt / 4);
        }
    }
    if (island.leaving.count) {
        leavingAlpha -= dt / kFadeOut.response;
        if (leavingAlpha <= 0) {
            leavingAlpha = 0;
            for (NSView *v in island.leaving) {
                [v removeFromSuperview];
            }
            [island.leaving removeAllObjects];
        }
    }
    [island paint];
    if (allRest()) {
        [frameTimer invalidate];
        frameTimer = nil;
        layoutWindow(YES);
        [island paint];
        if (hidden) {
            [panel orderOut:nil];
        }
    }
}

static void kick(void) {
    if (frameTimer) {
        return;
    }
    lastFrame = CACurrentMediaTime();
    frameTimer = [NSTimer timerWithTimeInterval:1.0 / 120 repeats:YES block:^(NSTimer *t) {
        (void)t;
        frame();
    }];
    [[NSRunLoop mainRunLoop] addTimer:frameTimer forMode:NSRunLoopCommonModes];
}

// tickClocks writes the countdowns; it runs once a second only while one shows.
static void tickClocks(void) {
    if (island.timers.count == 0) {
        [clockTimer invalidate];
        clockTimer = nil;
        return;
    }
    for (NSTextField *t in island.timers) {
        NSNumber *end = objc_getAssociatedObject(t, "end");
        t.stringValue = countdown(end.longLongValue);
    }
}

static void startClocks(void) {
    if (clockTimer || island.timers.count == 0) {
        return;
    }
    clockTimer = [NSTimer timerWithTimeInterval:1 repeats:YES block:^(NSTimer *t) {
        (void)t;
        tickClocks();
    }];
    [[NSRunLoop mainRunLoop] addTimer:clockTimer forMode:NSRunLoopCommonModes];
}

static void haptic(NSString *kind) {
    NSHapticFeedbackPattern p = [kind isEqualToString:@"alignment"] ? NSHapticFeedbackPatternAlignment : NSHapticFeedbackPatternLevelChange;
    [[NSHapticFeedbackManager defaultPerformer] performFeedbackPattern:p performanceTime:NSHapticFeedbackPerformanceTimeNow];
}

// watchClicks closes an open panel on a click in another app.
static void watchClicks(BOOL on) {
    if (on && !clickMonitor) {
        clickMonitor = [NSEvent addGlobalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown | NSEventMaskRightMouseDown
                                                              handler:^(NSEvent *e) {
                                                                  (void)e;
                                                                  if (island.panelShown) {
                                                                      [island dismiss];
                                                                  }
                                                              }];
    } else if (!on && clickMonitor) {
        [NSEvent removeMonitor:clickMonitor];
        clickMonitor = nil;
    }
}

static void apply(NSString *spec) {
    if (!panel) {
        return;
    }
    if (spec.length == 0) {
        hidden = YES;
        island.island = nil;
        island.panelShown = island.hoverOpen = NO;
        [island show:[[FlipView alloc] initWithFrame:NSZeroRect] size:NSZeroSize fade:YES];
        [island aimWidth:hasNotch ? notchW : 160 height:hasNotch ? notchH : 0 radius:10 params:kClose];
        springTo(&O, hasNotch ? 0 : 1, kClose);
        watchClicks(NO);
        return;
    }
    NSDictionary *d = [NSJSONSerialization JSONObjectWithData:[spec dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
    if (![d isKindOfClass:[NSDictionary class]]) {
        return;
    }
    NSDictionary *old = island.island;
    BOOL same = old && [sval(old, @"state") isEqualToString:sval(d, @"state")] && ival(old, @"notice") == ival(d, @"notice") &&
                [old[@"open"] boolValue] == [d[@"open"] boolValue];
    BOOL wasOpen = island.panelShown;
    BOOL open = [d[@"open"] boolValue];
    island.island = d;
    if (hidden) {
        hidden = NO;
        springJump(&O, 1);
        springTo(&O, 1, kUser);
        [panel orderFrontRegardless];
    }
    NSString *hk = sval(d, @"haptic");
    if (hk.length) {
        haptic(hk);
    }
    [autoTimer invalidate];
    autoTimer = nil;
    long long secs = ival(d, @"auto_close");
    if (open && secs > 0) {
        long long nid = ival(d, @"notice");
        autoTimer = [NSTimer scheduledTimerWithTimeInterval:secs repeats:NO block:^(NSTimer *t) {
            (void)t;
            autoTimer = nil;
            notchSend([NSString stringWithFormat:@"notch:expire:%lld", nid]);
        }];
    }
    if (same) {
        // the same moment or state with new figures: redraw in place
        if (!open && !island.hoverOpen) {
            island.panelShown = NO;
        }
        [island rebuildFade:NO params:kUser];
    } else {
        island.hoverOpen = NO;
        island.panelShown = open;
        island.trackAt = 0;
        SpringParams p = open ? kEvent : (wasOpen ? kClose : kEvent);
        [island rebuildFade:YES params:p];
    }
    watchClicks(island.panelShown);
    startClocks();
    tickClocks();
    kick();
}

static void setup(void) {
    pickScreen();
    reduceMotion = [[NSWorkspace sharedWorkspace] accessibilityDisplayShouldReduceMotion];
    panel = [[NotchPanel alloc] initWithContentRect:NSMakeRect(0, 0, 10, 10)
                                          styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
                                            backing:NSBackingStoreBuffered
                                              defer:NO];
    panel.floatingPanel = YES; // before the level: it sets the floating level
    panel.level = NSMainMenuWindowLevel + 2;
    panel.opaque = NO;
    panel.backgroundColor = [NSColor clearColor];
    panel.hasShadow = NO;
    panel.hidesOnDeactivate = NO;
    panel.becomesKeyOnlyIfNeeded = NO;
    panel.movable = NO;
    panel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary |
                               NSWindowCollectionBehaviorFullScreenAuxiliary | NSWindowCollectionBehaviorIgnoresCycle;
    panel.sharingType = NSWindowSharingNone;
    island = [[Island alloc] initWithFrame:NSMakeRect(0, 0, 10, 10)];
    panel.contentView = island;
    springJump(&W, hasNotch ? notchW : 160);
    springJump(&H, hasNotch ? notchH : 0);
    springJump(&R, 10);
    springJump(&O, 1);
    springJump(&CO, 1);
    layoutWindow(YES);
    NSNotificationCenter *nc = [NSNotificationCenter defaultCenter];
    [nc addObserverForName:NSApplicationDidChangeScreenParametersNotification
                    object:nil
                     queue:[NSOperationQueue mainQueue]
                usingBlock:^(NSNotification *n) {
                    (void)n;
                    pickScreen();
                    NSDictionary *d = island.island;
                    island.island = nil;
                    if (d) {
                        apply([[NSString alloc] initWithData:[NSJSONSerialization dataWithJSONObject:d options:0 error:nil]
                                                    encoding:NSUTF8StringEncoding]);
                    }
                    layoutWindow(YES);
                }];
    [[[NSWorkspace sharedWorkspace] notificationCenter]
        addObserverForName:NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification
                    object:nil
                     queue:[NSOperationQueue mainQueue]
                usingBlock:^(NSNotification *n) {
                    (void)n;
                    reduceMotion = [[NSWorkspace sharedWorkspace] accessibilityDisplayShouldReduceMotion];
                }];
}

void notch_setup(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        setup();
        goNotch(panel != nil);
    });
}

void bar_island(const char *spec) {
    NSString *s = spec ? [NSString stringWithUTF8String:spec] : @"";
    if (!s) {
        s = @"";
    }
    dispatch_async(dispatch_get_main_queue(), ^{
        apply(s);
    });
}

void bar_notch_sharing(int visible) {
    dispatch_async(dispatch_get_main_queue(), ^{
        panel.sharingType = visible ? NSWindowSharingReadOnly : NSWindowSharingNone;
    });
}
