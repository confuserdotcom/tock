// The notch panel: tock's island, drawn over the notch of the Mac. A display
// without a notch gets one drawn at the top of its menu bar, so every
// display has the same island; the one under the pointer answers. Go
// decides what it shows (bar.Island, as JSON); this side only lays it out,
// moves it and sends clicks back through goMenu.
//
// Motion follows the approved prototype: every size is a spring with a
// response (seconds) and a damping ratio, stepped by hand so that a new
// target always starts from where the shape is and how fast it moves.
//
// Behaviour follows the notch apps people already know:
//   - resting the pointer on the notch grows it a little (and ticks the
//     trackpad); a click or a two-finger swipe down opens it, or the rest
//     itself with [notch] open_on_hover;
//   - a quick pass of the pointer does nothing;
//   - an open panel goes back on Esc, a swipe up, a click on its top band
//     or anywhere else, and once the pointer has been in it and left;
//   - a moment tock opens stays while it is pointed at, else folds after
//     its time; nobody there, a session end or recap waits as a dot;
//   - in full screen a drawn notch steps aside while nothing runs.
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
static const SpringParams kOpen = {0.40, 0.86};  // you click it open: less
static const SpringParams kUser = {0.32, 1.0};   // you point at it: none
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
            @"deep" : hexc(0x4fa89a, 1),
            @"shallow" : hexc(0x6f86b5, 1),
            @"browse" : hexc(0xa07fb0, 1),
            @"neutral" : hexc(0x7d8088, 1),
            @"distract" : hexc(0xc9625a, 1),
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

static BOOL bval(NSDictionary *d, NSString *k) {
    id v = d[k];
    return [v isKindOfClass:[NSNumber class]] && [v boolValue];
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

// faceEmpty reports a face with nothing to show: the bare notch.
static BOOL faceEmpty(NSDictionary *f) {
    for (NSString *k in @[ @"dot", @"left", @"left_dim", @"right", @"right_dim" ]) {
        if (sval(f, k).length) {
            return NO;
        }
    }
    return ival(f, @"end_ms") == 0;
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

static void pillDone(NSWindow *w);

// Pill is a panel button or a label chip: a rounded fill, a title, an
// optional dot and key hint. Clicks send its ident.
@interface Pill : NSControl
@property(copy) NSString *ident;
@property(copy) NSString *title;
@property(copy) NSString *hint;  // "1", "S": shown as a key cap
@property(copy) NSString *extra; // "86%": dimmed after the title
@property(strong) NSColor *dot;
@property BOOL primary, chip, hover, pressed;
@end

@implementation Pill
- (BOOL)isFlipped {
    return YES;
}
// The panel never makes tockbar the active app: without this the first
// click would only make the panel key.
- (BOOL)acceptsFirstMouse:(NSEvent *)e {
    (void)e;
    return YES;
}
- (NSDictionary *)attrs {
    NSColor *c = self.primary ? [NSColor blackColor] : white(1);
    return @{NSFontAttributeName : sys(12, NSFontWeightMedium), NSForegroundColorAttributeName : c};
}
- (CGFloat)fitWidth {
    CGFloat w = [self.title sizeWithAttributes:[self attrs]].width + 22;
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
    [super updateTrackingAreas];
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
    self.hover = self.pressed = NO;
    self.needsDisplay = YES;
}
- (void)mouseDown:(NSEvent *)e {
    (void)e;
    [self.window makeFirstResponder:self.window.contentView]; // keys go to the island
    self.pressed = self.ident.length > 0;
    self.needsDisplay = YES;
}
- (void)mouseUp:(NSEvent *)e {
    NSPoint p = [self convertPoint:e.locationInWindow fromView:nil];
    BOOL was = self.pressed;
    self.pressed = NO;
    self.needsDisplay = YES;
    if (was && NSPointInRect(p, self.bounds)) {
        notchSend(self.ident);
        pillDone(self.window);
    }
}
- (void)drawRect:(NSRect)r {
    (void)r;
    NSRect b = self.bounds;
    CGFloat rad = self.chip ? b.size.height / 2 : 7;
    double lift = self.pressed ? 0.1 : (self.hover ? 0.08 : 0);
    NSColor *fill = self.primary ? [NSColor colorWithWhite:0.95 + (self.hover && !self.pressed ? 0.05 : 0) - (self.pressed ? 0.12 : 0) alpha:1]
                                 : white((self.chip ? 0.08 : 0.12) + lift);
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

// Bars is a stacked bar chart: one column per day or hour, parts by kind
// from the bottom, the label (and a sub line) under each column.
@interface Bars : FlipView
@property(strong) NSArray<NSDictionary *> *bars;
@end
@implementation Bars
+ (CGFloat)heightFor:(NSArray *)bars {
    BOOL sub = NO;
    for (NSDictionary *b in bars) {
        sub = sub || sval(b, @"sub").length > 0;
    }
    return 48 + 15 + (sub ? 13 : 0);
}
- (void)drawRect:(NSRect)r {
    (void)r;
    NSUInteger n = self.bars.count;
    if (!n) {
        return;
    }
    CGFloat w = self.bounds.size.width, ch = 48;
    CGFloat slot = w / n, cw = MIN(MAX(slot - 6, 3), 30);
    double top = 1;
    for (NSDictionary *b in self.bars) {
        double t = 0;
        for (NSDictionary *pt in aval(b, @"parts")) {
            t += ival(pt, @"ms");
        }
        top = MAX(top, t);
    }
    NSDictionary *la = @{NSFontAttributeName : digits(10, NSFontWeightRegular), NSForegroundColorAttributeName : white(0.45)};
    NSDictionary *ha = @{NSFontAttributeName : digits(10, NSFontWeightSemibold), NSForegroundColorAttributeName : white(0.95)};
    CGFloat lw = [@"Mon" sizeWithAttributes:la].width + 4;
    NSUInteger every = MAX((NSUInteger)ceil(lw / slot), 1u);
    for (NSUInteger i = 0; i < n; i++) {
        NSDictionary *b = self.bars[i];
        CGFloat x = floor(i * slot + (slot - cw) / 2);
        // the empty column, so a day with nothing still has its place
        [white(0.06) setFill];
        [[NSBezierPath bezierPathWithRoundedRect:NSMakeRect(x, 0, cw, ch) xRadius:2 yRadius:2] fill];
        double t = 0;
        for (NSDictionary *pt in aval(b, @"parts")) {
            t += ival(pt, @"ms");
        }
        CGFloat h = round(ch * t / top);
        if (h > 0) {
            [NSGraphicsContext saveGraphicsState];
            [[NSBezierPath bezierPathWithRoundedRect:NSMakeRect(x, ch - h, cw, h) xRadius:2 yRadius:2] addClip];
            CGFloat y = ch;
            for (NSDictionary *pt in aval(b, @"parts")) {
                CGFloat ph = h * ival(pt, @"ms") / t;
                [kindColor(sval(pt, @"kind")) setFill];
                NSRectFill(NSMakeRect(x, y - ph, cw, ph + 0.5));
                y -= ph;
            }
            [NSGraphicsContext restoreGraphicsState];
        }
        BOOL hi = bval(b, @"hi");
        if (i % every == 0 || hi) {
            NSString *l = sval(b, @"label");
            NSSize ls = [l sizeWithAttributes:la];
            [l drawAtPoint:NSMakePoint(round(i * slot + (slot - ls.width) / 2), ch + 3) withAttributes:hi ? ha : la];
        }
        NSString *sub = sval(b, @"sub");
        if (sub.length) {
            NSDictionary *sa = @{NSFontAttributeName : digits(9.5, NSFontWeightRegular), NSForegroundColorAttributeName : white(hi ? 0.75 : 0.4)};
            NSSize ss = [sub sizeWithAttributes:sa];
            [sub drawAtPoint:NSMakePoint(round(i * slot + (slot - ss.width) / 2), ch + 16) withAttributes:sa];
        }
    }
}
@end

// Rings are progress rings side by side: label, value and a sub line under
// each.
@interface Rings : FlipView
@property(strong) NSArray<NSDictionary *> *rings;
@end
@implementation Rings
+ (CGFloat)heightFor:(NSArray *)rings {
    BOOL sub = NO;
    for (NSDictionary *g in rings) {
        sub = sub || sval(g, @"sub").length > 0;
    }
    return 46 + 6 + 15 + 15 + (sub ? 13 : 0);
}
- (void)drawRect:(NSRect)r {
    (void)r;
    NSUInteger n = self.rings.count;
    if (!n) {
        return;
    }
    CGFloat slot = self.bounds.size.width / MAX(n, 2u), d = 46, lw = 5;
    for (NSUInteger i = 0; i < n; i++) {
        NSDictionary *g = self.rings[i];
        CGFloat cx = i * slot + slot / 2, cy = d / 2;
        NSBezierPath *track = [NSBezierPath bezierPath];
        [track appendBezierPathWithArcWithCenter:NSMakePoint(cx, cy) radius:(d - lw) / 2 startAngle:0 endAngle:360];
        track.lineWidth = lw;
        [white(0.12) setStroke];
        [track stroke];
        double f = MIN(MAX([g[@"frac"] doubleValue], 0), 1);
        if (f > 0) {
            // clockwise from 12 o'clock; the view is flipped
            NSBezierPath *arc = [NSBezierPath bezierPath];
            [arc appendBezierPathWithArcWithCenter:NSMakePoint(cx, cy) radius:(d - lw) / 2 startAngle:-90 endAngle:-90 + 360 * f clockwise:NO];
            arc.lineWidth = lw;
            arc.lineCapStyle = NSLineCapStyleRound;
            [kindColor(sval(g, @"kind")) setStroke];
            [arc stroke];
        }
        NSMutableParagraphStyle *ps = [NSMutableParagraphStyle new];
        ps.alignment = NSTextAlignmentCenter;
        ps.lineBreakMode = NSLineBreakByTruncatingTail;
        CGFloat y = d + 6;
        NSArray *lines = @[
            @[ sval(g, @"label"), sys(11, NSFontWeightRegular), white(0.62) ],
            @[ sval(g, @"value"), digits(12, NSFontWeightMedium), white(0.95) ],
            @[ sval(g, @"sub"), sys(10, NSFontWeightRegular), white(0.45) ],
        ];
        for (NSArray *ln in lines) {
            if ([ln[0] length]) {
                [ln[0] drawInRect:NSMakeRect(i * slot + 2, y, slot - 4, 15)
                   withAttributes:@{NSFontAttributeName : ln[1], NSForegroundColorAttributeName : ln[2], NSParagraphStyleAttributeName : ps}];
            }
            y += 15;
        }
    }
}
@end

// PageDots shows which page of the open notch is in view.
@interface PageDots : FlipView
@property NSInteger count, at;
@end
@implementation PageDots
- (void)drawRect:(NSRect)r {
    (void)r;
    if (self.count < 2) {
        return;
    }
    CGFloat d = 5, gap = 7, w = self.count * d + (self.count - 1) * gap;
    CGFloat x = (self.bounds.size.width - w) / 2;
    for (NSInteger i = 0; i < self.count; i++) {
        [white(i == self.at ? 0.9 : 0.28) setFill];
        [[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(x + i * (d + gap), 1, d, d)] fill];
    }
}
@end

// ---------- shared state ----------

@class Island;

static NSMutableArray<Island *> *islands; // one per display
static Island *active;                    // where tock's moments open
static NSDictionary *spec;                // what Go asked for; nil = hidden
static BOOL optSharing, optAll = YES, optHover;
static BOOL reduceMotion;

// The open quiet panel has three sizes you drag or pinch between: S, M and
// L. The size and the page in view are remembered per Mac (UI state, not
// config).
static NSString *const kSizeKey = @"NotchPanelSize";
static NSString *const kPageKey = @"NotchPanelPage";
static const CGFloat kLevelW[3] = {440, 580, 740};
static const CGFloat kGrip = 18; // the resize grips at the bottom corners
static NSInteger sizeLevel = -1;
static NSInteger panelSize(void) {
    if (sizeLevel < 0) {
        sizeLevel = MIN(MAX([[NSUserDefaults standardUserDefaults] integerForKey:kSizeKey], 0), 2);
    }
    return sizeLevel;
}
static void setPanelSize(NSInteger l) {
    sizeLevel = MIN(MAX(l, 0), 2);
    [[NSUserDefaults standardUserDefaults] setInteger:sizeLevel forKey:kSizeKey];
}

// rubber is how far past an end a drag of d goes: less and less.
static double rubber(double d, double scale) { return scale * d / (d + scale) * 0.55; }

static NSCursor *resizeCursor(int side) {
    if (@available(macOS 15.0, *)) {
        return [NSCursor frameResizeCursorFromPosition:side > 0 ? NSCursorFrameResizePositionBottomRight : NSCursorFrameResizePositionBottomLeft
                                          inDirections:NSCursorFrameResizeDirectionsAll];
    }
    return [NSCursor crosshairCursor];
}
static NSTimer *frameTimer, *clockTimer, *autoTimer;
static id clickMonitor;
static CFTimeInterval lastFrame;

static const CGFloat kEar = 8;      // the fillets that join the menu bar
static const CGFloat kPad = 18;     // panel side padding
static const CGFloat kGap = 6;      // between the face band and the body
static const CGFloat kMaxW = 960;   // the window never needs more
static const CGFloat kMaxH = 640;
static const CGFloat kSlop = 24;    // room for overshoot while moving
static const CGFloat kDrawnW = 185; // a drawn notch with no real one to copy
static const CGFloat kPeekW = 18;   // how much a pointed-at notch grows
static const CGFloat kPeekH = 3;
static const double kIntent = 0.10;   // the rest that counts as pointing
static const double kQuickPass = 900; // pt/s: faster is passing by

static void kick(void);
static void watchClicks(void);
static void renderAll(SpringParams p);

static BOOL momentOpen(void) { return bval(spec, @"open"); }

static void haptic(NSHapticFeedbackPattern p) {
    [[NSHapticFeedbackManager defaultPerformer] performFeedbackPattern:p performanceTime:NSHapticFeedbackPerformanceTimeNow];
}

// ---------- the island ----------

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

@interface Island : FlipView {
  @public
    Spring W, H, R, O, CO, CY, TX, G; // width, height, radius, opacity, content opacity, content y, track x, grips
    double leavingAlpha;
    // swiping pages: where the fingers put the track, unbounded, and its speed
    BOOL hswipe;
    double swipeRaw, swipeV;
    CFTimeInterval swipeT;
    // resizing by a grip: where the drag started and the size then
    NSPoint dragAt;
    CGSize dragFrom;
    int dragSide;
    double dragVW, dragVH, dragEq, dragVE;
    CFTimeInterval dragT;
    double pinch;
    BOOL pinchFired;
    // dragging pages with the mouse: pressed on a page, where the track was
    BOOL pageArmed;
    double pageBase;
    double wheel;
    BOOL wheelFired;
    NSPoint lastP;
    CFTimeInterval lastT;
    double speed;
}
@property(strong) NotchPanel *win;
@property NSRect screenFrame;
@property CGDirectDisplayID displayID; // not display: that is NSView's redraw
@property BOOL real;      // a camera cutout: the island hides behind it at rest
@property BOOL barHidden; // its menu bar is away (full screen)
@property CGFloat notchW, notchH, midX, top;
@property(strong) NSView *content; // the content shown, fading in
@property(strong) NSMutableArray<NSView *> *leaving;
@property(strong) NSMutableArray<NSTextField *> *timers; // countdown labels
@property(strong) NSView *track;                         // labelling cards
@property NSInteger trackAt;                             // the card the track shows
@property(copy) NSString *look;                          // what shows, to cross-fade on change
@property BOOL panelShown;                               // the panel, not the face
@property BOOL userOpen, byHover;                        // opened by you, by resting on it
@property BOOL peek;                                     // pointed at while closed: a little bigger
@property BOOL inside, intent;                           // the pointer is on it; it rested there
@property BOOL engaged;                                  // the pointer rested on the open moment
@property(strong) NSTimer *intentTimer, *leaveTimer, *slideTimer;
@property CGSize target;
@property NSSize contentSize;
@property(strong) PageDots *dots;      // the pages' dots; nil = no pages
@property NSInteger pageCount, pageAt; // the pages, the one in view
@property(strong) NSArray<NSString *> *pageIDs;
@property BOOL resizing;      // a grip is being dragged
@property NSInteger dragLevel; // the size the content is laid out for meanwhile
@property BOOL gripCursor;
- (void)render:(SpringParams)p;
- (void)paint;
- (void)layoutWindow:(BOOL)settle;
- (BOOL)atRest;
- (void)step:(double)dt;
- (void)closeUser:(SpringParams)p;
- (void)dismiss;
- (void)teardown;
@end

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

- (BOOL)momentHere {
    return momentOpen() && self == active;
}

// panelDict is the panel to show here: tock's moment, or the one you opened.
- (NSDictionary *)panelDict {
    if (!spec) {
        return nil;
    }
    if (self.momentHere) {
        return dval(spec, @"panel");
    }
    return self.userOpen ? dval(spec, @"hover") : nil;
}

// ----- geometry -----

- (CGFloat)ear {
    if (self.real) { // no fillets while it hides behind the camera
        return MIN(MAX((W.x - self.notchW) / 2, 0), kEar);
    }
    return MIN(MAX(4 + (W.x - self.notchW) / 4, 4), kEar);
}

// shapeRect is the island in view coordinates: centred, from the top.
- (NSRect)shapeRect {
    CGFloat w = MAX(W.x, 0), h = MAX(H.x, 0);
    return NSMakeRect((self.bounds.size.width - w) / 2, 0, w, h);
}

- (BOOL)inShape:(NSPoint)p {
    NSRect s = NSInsetRect([self shapeRect], -[self ear] - 2, 0);
    s.size.height += 3;
    if (self.real) { // at rest the island hides inside the camera: the camera counts
        s = NSUnionRect(s, NSMakeRect((self.bounds.size.width - self.notchW) / 2, 0, self.notchW, self.notchH));
    }
    return NSPointInRect(p, s);
}

- (void)drawRect:(NSRect)dirty {
    (void)dirty;
    NSRect s = [self shapeRect];
    if (s.size.width < 1 || s.size.height < 0.5) {
        return;
    }
    CGFloat r = MIN(MAX(R.x, 0), MIN(s.size.width / 2, s.size.height));
    [[NSColor colorWithWhite:0 alpha:MIN(MAX(O.x, 0), 1)] setFill];
    NSBezierPath *p = [NSBezierPath bezierPath];
    CGFloat x0 = NSMinX(s), x1 = NSMaxX(s), y1 = NSMaxY(s);
    // a flat top that runs into the menu bar through two concave fillets,
    // round corners at the bottom
    CGFloat e = [self ear], k = 0.45, ey = MIN(e, y1 - r);
    ey = MAX(ey, 0);
    [p moveToPoint:NSMakePoint(x0 - e, 0)];
    [p lineToPoint:NSMakePoint(x1 + e, 0)];
    [p curveToPoint:NSMakePoint(x1, ey) controlPoint1:NSMakePoint(x1 + e * k, 0) controlPoint2:NSMakePoint(x1, ey * (1 - k))];
    [p lineToPoint:NSMakePoint(x1, y1 - r)];
    [p curveToPoint:NSMakePoint(x1 - r, y1) controlPoint1:NSMakePoint(x1, y1 - r * 0.45) controlPoint2:NSMakePoint(x1 - r * 0.45, y1)];
    [p lineToPoint:NSMakePoint(x0 + r, y1)];
    [p curveToPoint:NSMakePoint(x0, y1 - r) controlPoint1:NSMakePoint(x0 + r * 0.45, y1) controlPoint2:NSMakePoint(x0, y1 - r * 0.45)];
    [p lineToPoint:NSMakePoint(x0, ey)];
    [p curveToPoint:NSMakePoint(x0 - e, 0) controlPoint1:NSMakePoint(x0, ey * (1 - k)) controlPoint2:NSMakePoint(x0 - e * k, 0)];
    [p closePath];
    [p fill];
    // the resize grips: a quarter arc inside each bottom corner
    CGFloat ga = MIN(MAX(G.x, 0), 1) * MIN(MAX(O.x, 0), 1);
    if (ga > 0.01 && s.size.height > 80) {
        CGFloat rr = MAX(r - 6, 6);
        for (int side = -1; side <= 1; side += 2) {
            NSPoint c = NSMakePoint(side > 0 ? x1 - r : x0 + r, y1 - r);
            NSBezierPath *a = [NSBezierPath bezierPath];
            [a appendBezierPathWithArcWithCenter:c radius:rr startAngle:side > 0 ? 20 : 110 endAngle:side > 0 ? 70 : 160];
            a.lineWidth = 2;
            a.lineCapStyle = NSLineCapStyleRound;
            [white(0.4 * ga) setStroke];
            [a stroke];
        }
    }
}

// ----- pages and sizes -----

- (BOOL)pagesShown {
    return self.panelShown && self.pageCount > 0 && !self.momentHere;
}

- (NSInteger)level {
    return self.resizing ? self.dragLevel : panelSize();
}

- (CGFloat)levelWidth:(NSInteger)l {
    CGFloat w = MAX(kLevelW[MIN(MAX(l, 0), 2)], self.notchW + 220);
    return MIN(w, self.screenFrame.size.width * 0.9);
}

// gripAt is the grip under p: 1 the right one, -1 the left one, 0 none.
- (int)gripAt:(NSPoint)p {
    if (!self.pagesShown) {
        return 0;
    }
    NSRect s = [self shapeRect];
    if (p.y < NSMaxY(s) - kGrip - 4 || p.y > NSMaxY(s) + 3) {
        return 0;
    }
    if (p.x >= NSMaxX(s) - kGrip - 4 && p.x <= NSMaxX(s) + 3) {
        return 1;
    }
    if (p.x <= NSMinX(s) + kGrip + 4 && p.x >= NSMinX(s) - 3) {
        return -1;
    }
    return 0;
}

// aimGrips shows the grips while the pointer is on a panel with pages.
- (void)aimGrips {
    double t = self.pagesShown && (self.inside || self.resizing) ? 1 : 0;
    if (G.t != t) {
        if (reduceMotion) {
            springJump(&G, t);
        } else {
            springTo(&G, t, kFade);
        }
        kick();
    }
}

// goPage settles the track on page to, carrying the swipe's speed v.
- (void)goPage:(NSInteger)to velocity:(double)v {
    if (self.pageCount <= 0) { // the pages went away under the gesture
        return;
    }
    to = MIN(MAX(to, 0), self.pageCount - 1);
    if (to != self.pageAt) {
        haptic(NSHapticFeedbackPatternAlignment);
        self.pageAt = to;
        if (to < (NSInteger)self.pageIDs.count) {
            [[NSUserDefaults standardUserDefaults] setObject:self.pageIDs[to] forKey:kPageKey];
        }
    }
    if (reduceMotion) {
        springJump(&TX, to);
    } else {
        TX.v = v;
        springTo(&TX, to, kSlide);
    }
    kick();
}

- (void)mouseDragged:(NSEvent *)e {
    if (pageArmed) {
        [self dragPages:e];
        return;
    }
    if (!self.resizing) {
        return;
    }
    // like scaling a picture: the corner sets one scale for both sides
    NSPoint m = [NSEvent mouseLocation];
    CGFloat dx = (m.x - dragAt.x) * dragSide, dy = dragAt.y - m.y;
    double scale = MAX((dragFrom.width + 2 * dx) / MAX(dragFrom.width, 1), (dragFrom.height + dy) / MAX(dragFrom.height, 1));
    CGFloat minW = [self levelWidth:0] * 0.9, maxW = MIN(self.screenFrame.size.width * 0.9, kMaxW - kEar * 2 - kSlop);
    CGFloat minH = self.notchH + 90, maxH = MIN(self.screenFrame.size.height * 0.45, kMaxH - kSlop);
    CGFloat w = dragFrom.width * scale, h = dragFrom.height * scale;
    if (w < minW) {
        w = minW - rubber(minW - w, 60);
    } else if (w > maxW) {
        w = maxW + rubber(w - maxW, 60);
    }
    if (h < minH) {
        h = minH - rubber(minH - h, 40);
    } else if (h > maxH) {
        h = maxH + rubber(h - maxH, 40);
    }
    double eq = dragFrom.width * scale, dt = e.timestamp - dragT;
    if (dt > 0) {
        dragVW = 0.6 * dragVW + 0.4 * (w - W.x) / dt;
        dragVH = 0.6 * dragVH + 0.4 * (h - H.x) / dt;
        dragVE = 0.6 * dragVE + 0.4 * (eq - dragEq) / dt;
    }
    dragT = e.timestamp;
    dragEq = eq;
    springJump(&W, w);
    springJump(&H, h);
    self.target = CGSizeMake(w, h);
    NSInteger lv = 0;
    for (NSInteger l = 1; l < 3; l++) {
        if (fabs([self levelWidth:l] - eq) < fabs([self levelWidth:lv] - eq)) {
            lv = l;
        }
    }
    if (lv != self.dragLevel) { // lay the content out for the size it nears
        self.dragLevel = lv;
        haptic(NSHapticFeedbackPatternAlignment);
        [self render:kUser];
    }
    [self layoutWindow:NO];
    [self paint];
}

- (void)mouseUp:(NSEvent *)e {
    if (pageArmed) {
        pageArmed = NO;
        if (hswipe) { // let go of a page drag: settle like a swipe
            hswipe = NO;
            [[NSCursor arrowCursor] set];
            double proj = swipeRaw + swipeV * 0.2;
            NSInteger to = lround(MIN(MAX(proj, self.pageAt - 1), self.pageAt + 1));
            [self goPage:to velocity:MAX(MIN(swipeV, 12), -12)];
            [self setPointerInside:[self inShape:[self convertPoint:e.locationInWindow fromView:nil]]];
            if (!self.inside) {
                [self armLeave];
            }
        }
        return;
    }
    if (!self.resizing) {
        return;
    }
    // settle on the size nearest to where the drag was heading
    double proj = dragEq + dragVE * 0.2;
    NSInteger lv = 0;
    for (NSInteger l = 1; l < 3; l++) {
        if (fabs([self levelWidth:l] - proj) < fabs([self levelWidth:lv] - proj)) {
            lv = l;
        }
    }
    self.resizing = NO;
    setPanelSize(lv);
    W.v = MAX(MIN(dragVW, 3000), -3000);
    H.v = MAX(MIN(dragVH, 3000), -3000);
    [self render:kUser];
    [self setPointerInside:[self inShape:[self convertPoint:e.locationInWindow fromView:nil]]];
    [self aimGrips];
}

// A pinch on the open panel steps its size, one size per pinch.
- (void)magnifyWithEvent:(NSEvent *)e {
    if (!self.pagesShown || self.resizing) {
        return;
    }
    if (e.phase == NSEventPhaseBegan) {
        pinch = 0;
        pinchFired = NO;
    }
    pinch += e.magnification;
    if (!pinchFired && fabs(pinch) > 0.18) {
        pinchFired = YES;
        NSInteger l = panelSize() + (pinch > 0 ? 1 : -1);
        if (l >= 0 && l <= 2) {
            setPanelSize(l);
            haptic(NSHapticFeedbackPatternAlignment);
            [self render:kUser];
        }
    }
}

// Clicks on text go to the island; buttons keep theirs.
- (NSView *)hitTest:(NSPoint)pt {
    NSView *v = [super hitTest:pt];
    if (!v) {
        return nil;
    }
    for (NSView *x = v; x && x != self; x = x.superview) {
        if ([x isKindOfClass:[Pill class]]) {
            return x;
        }
    }
    return self;
}

// ----- the pointer -----

- (void)updateTrackingAreas {
    [super updateTrackingAreas];
    for (NSTrackingArea *a in self.trackingAreas) {
        [self removeTrackingArea:a];
    }
    [self addTrackingArea:[[NSTrackingArea alloc] initWithRect:NSZeroRect
                                                       options:NSTrackingMouseEnteredAndExited | NSTrackingMouseMoved | NSTrackingActiveAlways |
                                                               NSTrackingInVisibleRect
                                                         owner:self
                                                      userInfo:nil]];
}

- (void)pointer:(NSEvent *)e {
    NSPoint p = [self convertPoint:e.locationInWindow fromView:nil];
    double dt = e.timestamp - lastT;
    speed = (dt > 0 && dt < 0.2) ? hypot(p.x - lastP.x, p.y - lastP.y) / dt : 0;
    lastP = p;
    lastT = e.timestamp;
    [self setPointerInside:[self inShape:p]];
    int g = [self gripAt:p];
    if (g) {
        [resizeCursor(g) set];
        self.gripCursor = YES;
    } else if (self.gripCursor) {
        [[NSCursor arrowCursor] set];
        self.gripCursor = NO;
    }
}

- (void)mouseEntered:(NSEvent *)e {
    [self pointer:e];
}
- (void)mouseMoved:(NSEvent *)e {
    [self pointer:e];
}
- (void)mouseExited:(NSEvent *)e {
    (void)e;
    if (self.gripCursor && !self.resizing) {
        [[NSCursor arrowCursor] set];
        self.gripCursor = NO;
    }
    [self setPointerInside:NO];
}

- (void)setPointerInside:(BOOL)in {
    if (in == self.inside) {
        return;
    }
    self.inside = in;
    [self aimGrips];
    if (in) {
        [self.leaveTimer invalidate];
        self.leaveTimer = nil;
        [self armIntent:kIntent];
        return;
    }
    if (self.resizing || hswipe) { // the drag goes on outside; mouse-up decides
        return;
    }
    [self.intentTimer invalidate];
    self.intentTimer = nil;
    self.intent = NO;
    if (self.peek) {
        self.peek = NO;
        [self render:kUser];
    }
    [self armLeave];
}

// armIntent waits for the pointer to rest: a quick pass over the top of the
// screen, on its way to a menu, must not grow or open anything.
- (void)armIntent:(double)after {
    [self.intentTimer invalidate];
    __weak Island *me = self;
    self.intentTimer = [NSTimer scheduledTimerWithTimeInterval:after
                                                       repeats:NO
                                                         block:^(NSTimer *t) {
                                                             (void)t;
                                                             [me rested];
                                                         }];
}

- (void)rested {
    self.intentTimer = nil;
    if (!self.inside) {
        return;
    }
    if (speed > kQuickPass && CACurrentMediaTime() - lastT < 0.06) {
        [self armIntent:0.05];
        return;
    }
    self.intent = YES;
    if (self.panelShown) {
        if (self.momentHere) {
            self.engaged = YES; // seen: leaving puts it away
        }
        return;
    }
    if (!spec) {
        return;
    }
    if (optHover && dval(spec, @"hover")) {
        [self openUser:YES params:kUser];
        return;
    }
    if (dval(spec, @"hover") || momentOpen()) {
        self.peek = YES;
        haptic(NSHapticFeedbackPatternGeneric);
        [self render:kUser];
    }
}

// armLeave puts an open panel away once the pointer has left it: one you
// opened, or a moment you pointed at. Labelling stays: keys may go on.
- (void)armLeave {
    double after = 0;
    if (self.userOpen) {
        after = self.byHover ? 0.32 : 0.7;
    } else if (self.momentHere && self.engaged && ![sval(spec, @"state") isEqualToString:@"label"]) {
        after = 0.9;
    }
    if (after == 0) {
        return;
    }
    [self.leaveTimer invalidate];
    __weak Island *me = self;
    self.leaveTimer = [NSTimer scheduledTimerWithTimeInterval:after
                                                      repeats:NO
                                                        block:^(NSTimer *t) {
                                                            (void)t;
                                                            Island *s = me;
                                                            s.leaveTimer = nil;
                                                            if (!s || s.inside) {
                                                                return;
                                                            }
                                                            [s dismiss];
                                                        }];
}

// ----- opening and closing -----

// openUser opens the panel here: the moment, moved from another display,
// or the hover panel of the running state.
- (void)openUser:(BOOL)byHover params:(SpringParams)p {
    if (!spec) {
        return;
    }
    if (momentOpen()) {
        if (active != self) {
            Island *from = active;
            active = self;
            self.engaged = YES;
            [from render:kClose];
            [self render:kOpen];
            watchClicks();
        }
        return;
    }
    if (!dval(spec, @"hover")) {
        return;
    }
    for (Island *i in islands) {
        if (i != self && i.userOpen) {
            i.userOpen = NO;
            [i render:kClose];
        }
    }
    self.userOpen = YES;
    self.byHover = byHover;
    self.peek = NO;
    if (!bval(spec, @"pending")) {
        notchSend(@"notch:open"); // Go reads the pages while it is open
    }
    [self render:p];
    watchClicks();
}

// closeUser folds the panel you opened. A missed moment was seen.
- (void)closeUser:(SpringParams)p {
    if (!self.userOpen) {
        return;
    }
    self.userOpen = self.byHover = NO;
    self.resizing = NO;
    if (bval(spec, @"pending")) {
        notchSend(@"notch:seen");
    } else {
        notchSend(@"notch:shut");
    }
    [self render:p];
    watchClicks();
}

// dismiss sends what is open home; Go draws what comes next. The pointer
// counts as gone, so resting on it again grows it again.
- (void)dismiss {
    self.inside = self.intent = NO;
    [self.leaveTimer invalidate];
    self.leaveTimer = nil;
    if (self.momentHere) {
        self.engaged = NO;
        notchSend(@"notch:close");
    } else {
        [self closeUser:kClose];
    }
}

- (void)mouseDown:(NSEvent *)e {
    NSPoint p = [self convertPoint:e.locationInWindow fromView:nil];
    int g = [self gripAt:p];
    if (g) {
        if (e.clickCount == 2) { // back to the small size
            self.resizing = NO;
            setPanelSize(0);
            [self render:kUser];
            return;
        }
        self.resizing = YES;
        self.dragLevel = panelSize();
        dragSide = g;
        dragAt = [NSEvent mouseLocation];
        dragFrom = CGSizeMake(W.x, H.x);
        dragEq = W.x;
        dragVW = dragVH = dragVE = 0;
        dragT = e.timestamp;
        [self.leaveTimer invalidate];
        self.leaveTimer = nil;
        return;
    }
    if (![self inShape:p]) {
        return;
    }
    [self.window makeFirstResponder:self]; // Esc and the label keys come here
    if (self.pagesShown && p.y >= self.notchH) { // maybe a drag through the pages
        pageArmed = YES;
        pageBase = swipeRaw = TX.x;
        dragAt = [NSEvent mouseLocation];
        swipeV = 0;
        swipeT = e.timestamp;
        return;
    }
    if (!self.panelShown) {
        [self openUser:NO params:kOpen];
        return;
    }
    if (p.y < self.notchH) { // the top band, beside the camera: put it away
        [self dismiss];
    }
}

- (void)rightMouseDown:(NSEvent *)e {
    (void)e;
    if (self.panelShown) {
        [self dismiss];
    }
}

- (void)keyDown:(NSEvent *)e {
    NSString *c = e.charactersIgnoringModifiers;
    if (e.keyCode == 53) { // Esc
        [self dismiss];
        return;
    }
    if (self.pagesShown && (e.keyCode == 123 || e.keyCode == 124)) { // ← →
        [self goPage:self.pageAt + (e.keyCode == 124 ? 1 : -1) velocity:0];
        return;
    }
    if ([sval(spec, @"state") isEqualToString:@"label"] && self.momentHere) {
        NSDictionary *p = dval(spec, @"panel");
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

// Two fingers down on the notch open it, up on a panel send it home; a
// flick counts as much as a long swipe, and its speed carries into the
// height spring.
- (void)scrollWheel:(NSEvent *)e {
    if (e.momentumPhase != NSEventPhaseNone) {
        return;
    }
    if (hswipe || (self.pagesShown && self.track && e.phase != NSEventPhaseNone)) {
        if ([self swipe:e]) {
            return;
        }
    }
    if (![self inShape:[self convertPoint:e.locationInWindow fromView:nil]]) {
        return;
    }
    if (e.phase == NSEventPhaseBegan || e.phase == NSEventPhaseMayBegin) {
        wheel = 0;
        wheelFired = NO;
    }
    if (wheelFired) {
        return;
    }
    double d = e.scrollingDeltaY;
    if (!e.hasPreciseScrollingDeltas) {
        d *= 10; // a mouse wheel notch
    }
    if (!e.isDirectionInvertedFromDevice) {
        d = -d; // d > 0: the fingers moved down
    }
    wheel += d;
    BOOL flick = fabs(d) > 9;
    if (!self.panelShown && (wheel > 14 || (flick && d > 0))) {
        wheelFired = YES;
        [self openUser:NO params:kOpen];
        H.v = MIN(fabs(d) * 40, 1400);
    } else if (self.panelShown && (wheel < -22 || (flick && d < 0))) {
        wheelFired = YES;
        H.v = -MIN(fabs(d) * 40, 1400);
        [self dismiss];
    }
    if (e.phase == NSEventPhaseNone) { // a wheel has no gesture to end
        wheelFired = NO;
        wheel = 0;
    }
}

// endPageGesture drops a swipe, a page drag or a resize the content went
// away under.
- (void)endPageGesture {
    if (hswipe || pageArmed || self.resizing || self.gripCursor) {
        [[NSCursor arrowCursor] set];
    }
    hswipe = pageArmed = NO;
    self.resizing = self.gripCursor = NO;
}

// dragPages moves the pages with the mouse held down, as two fingers do: it
// starts once the pointer has gone a few points sideways.
- (void)dragPages:(NSEvent *)e {
    NSPoint m = [NSEvent mouseLocation];
    double dx = m.x - dragAt.x;
    if (!hswipe) {
        if (fabs(dx) < 4) {
            return;
        }
        hswipe = YES;
        [self.leaveTimer invalidate];
        self.leaveTimer = nil;
        [[NSCursor closedHandCursor] set];
    }
    double w = MAX(self.contentSize.width, 1);
    double raw = pageBase - dx / w, dt = e.timestamp - swipeT;
    if (dt > 0) {
        swipeV = 0.6 * swipeV + 0.4 * (raw - swipeRaw) / dt;
    }
    swipeT = e.timestamp;
    swipeRaw = raw;
    double hi = MAX(self.pageCount - 1, 0), x = raw;
    if (x < 0) {
        x = -rubber(-x, 0.5);
    } else if (x > hi) {
        x = hi + rubber(x - hi, 0.5);
    }
    springJump(&TX, x);
    [self paint];
}

// swipe pages with two fingers sideways: the track follows the fingers,
// stretches past the ends, and on release settles on the page the flick
// points to. Returns NO for a gesture that is not a sideways swipe.
- (BOOL)swipe:(NSEvent *)e {
    if (e.phase == NSEventPhaseBegan || e.phase == NSEventPhaseMayBegin) {
        hswipe = NO;
        return NO;
    }
    double dx = e.scrollingDeltaX, dy = e.scrollingDeltaY;
    if (!hswipe) {
        if (e.phase != NSEventPhaseChanged || wheelFired || fabs(dx) <= fabs(dy) || fabs(dx) < 1.5 ||
            ![self inShape:[self convertPoint:e.locationInWindow fromView:nil]]) {
            return NO;
        }
        hswipe = YES;
        swipeRaw = TX.x;
        swipeV = 0;
        swipeT = e.timestamp;
        [self.leaveTimer invalidate];
        self.leaveTimer = nil;
    }
    double w = MAX(self.contentSize.width, 1);
    if (e.phase == NSEventPhaseChanged) {
        double fdx = e.isDirectionInvertedFromDevice ? dx : -dx; // the fingers' move, + = right
        swipeRaw -= fdx / w;
        double dt = e.timestamp - swipeT;
        if (dt > 0) {
            swipeV = 0.6 * swipeV + 0.4 * (-fdx / w) / dt;
        }
        swipeT = e.timestamp;
        double hi = MAX(self.pageCount - 1, 0), x = swipeRaw;
        if (x < 0) {
            x = -rubber(-x, 0.5);
        } else if (x > hi) {
            x = hi + rubber(x - hi, 0.5);
        }
        springJump(&TX, x);
        [self paint];
        return YES;
    }
    hswipe = NO; // ended or cancelled
    double proj = swipeRaw + swipeV * 0.2;
    NSInteger to = lround(MIN(MAX(proj, self.pageAt - 1), self.pageAt + 1));
    [self goPage:to velocity:MAX(MIN(swipeV, 12), -12)];
    if (!self.inside) {
        [self armLeave];
    }
    return YES;
}

// ----- content -----

- (NSView *)wingsWidth:(CGFloat)w face:(NSDictionary *)f {
    CGFloat h = self.notchH;
    FlipView *v = [[FlipView alloc] initWithFrame:NSMakeRect(0, 0, w, h)];
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
    CGFloat wing = (w - self.notchW) / 2; // the room left of the camera
    if (left.length) {
        NSTextField *t = text(left, ft, ink);
        t.frame = NSMakeRect(x, (h - t.frame.size.height) / 2, MAX(0, MIN(t.frame.size.width, wing - x - 4)), t.frame.size.height);
        [v addSubview:t];
        x += t.frame.size.width + 6;
    }
    NSString *ld = sval(f, @"left_dim");
    if (ld.length && x < wing - 8) {
        NSTextField *t = text(ld, sys(13, NSFontWeightRegular), white(0.55));
        t.frame = NSMakeRect(x, (h - t.frame.size.height) / 2, MIN(t.frame.size.width, wing - x - 4), t.frame.size.height);
        [v addSubview:t];
    }
    CGFloat rx = w - 14;
    long long end = ival(f, @"end_ms");
    NSString *right = end > 0 ? countdown(end) : sval(f, @"right");
    NSString *rd = sval(f, @"right_dim");
    NSTextField *rt = nil;
    if (right.length) {
        NSFont *df = digits(13, NSFontWeightMedium);
        rt = text(right, df, ink);
        if (end > 0) {
            // as wide as the widest it gets, so the island does not twitch
            CGFloat cw = MAX(rt.frame.size.width, ceil([@"88:88" sizeWithAttributes:@{NSFontAttributeName : df}].width) + 2);
            rt.alignment = NSTextAlignmentRight;
            objc_setAssociatedObject(rt, "end", @(end), OBJC_ASSOCIATION_RETAIN);
            rt.frame = NSMakeRect(0, 0, cw, rt.frame.size.height);
            [self.timers addObject:rt];
        }
    }
    // the dim text sits after the value: "18:42 left", "in 2m" puts it before
    BOOL dimFirst = [rd isEqualToString:@"in"];
    NSTextField *dt = rd.length ? text(rd, sys(13, NSFontWeightRegular), white(0.55)) : nil;
    NSArray *order = dimFirst ? @[ rt ?: [NSNull null], dt ?: [NSNull null] ] : @[ dt ?: [NSNull null], rt ?: [NSNull null] ];
    CGFloat rightEdge = w - wing + 4; // never under the camera
    for (id o in order) {
        if (o == [NSNull null]) {
            continue;
        }
        NSTextField *t = o;
        CGFloat tw = MIN(t.frame.size.width, rx - rightEdge);
        if (tw <= 8) {
            break;
        }
        rx -= tw;
        t.frame = NSMakeRect(rx, (h - t.frame.size.height) / 2, tw, t.frame.size.height);
        [v addSubview:t];
        rx -= 5;
    }
    return v;
}

// faceWidth is the compact island's width: the notch plus a wing either
// side wide enough for the longer side.
- (CGFloat)faceWidth:(NSDictionary *)f {
    NSFont *ft = sys(13, NSFontWeightMedium);
    BOOL words = sval(f, @"left").length || sval(f, @"left_dim").length || sval(f, @"right").length || sval(f, @"right_dim").length ||
                 ival(f, @"end_ms") > 0;
    if (!words) { // a dot alone: a missed moment
        return self.notchW + 2 * 30;
    }
    CGFloat l = 14 + (sval(f, @"dot").length ? 14 : 0) + [sval(f, @"left") sizeWithAttributes:@{NSFontAttributeName : ft}].width;
    if (sval(f, @"left_dim").length) {
        l += 6 + [sval(f, @"left_dim") sizeWithAttributes:@{NSFontAttributeName : ft}].width;
    }
    NSString *right = ival(f, @"end_ms") > 0 ? @"88:88" : sval(f, @"right");
    CGFloat r = 14 + [right sizeWithAttributes:@{NSFontAttributeName : digits(13, NSFontWeightMedium)}].width;
    if (sval(f, @"right_dim").length) {
        r += 5 + [sval(f, @"right_dim") sizeWithAttributes:@{NSFontAttributeName : ft}].width;
    }
    CGFloat side = MIN(MAX(MAX(l, r) + 12, 64), 200);
    CGFloat room = (self.screenFrame.size.width - self.notchW) / 2 - 40; // leave the menus some menu bar
    return self.notchW + 2 * MIN(side, MAX(room, 64));
}

- (CGFloat)panelWidth:(NSString *)state {
    NSDictionary *w = @{@"done" : @580, @"recap" : @640, @"label" : @600, @"drift" : @540, @"meet" : @480};
    CGFloat v = [w[state] doubleValue];
    v = MAX(v ? v : 440, self.notchW + 220);
    return MIN(v, self.screenFrame.size.width - 24);
}

// addButtons lays pills out from x = kPad, wrapping; returns the height.
- (CGFloat)addButtons:(NSArray *)bs to:(NSView *)v y:(CGFloat)y width:(CGFloat)w {
    CGFloat x = kPad, rowH = 26, y0 = y;
    for (NSDictionary *b in bs) {
        Pill *p = [[Pill alloc] initWithFrame:NSZeroRect];
        p.ident = sval(b, @"id");
        p.title = sval(b, @"title");
        p.primary = bval(b, @"primary");
        CGFloat pw = MIN([p fitWidth], w - kPad * 2);
        if (x > kPad && x + pw > w - kPad) {
            x = kPad;
            y += rowH + 8;
        }
        p.frame = NSMakeRect(x, y, pw, rowH);
        [v addSubview:p];
        x += pw + 8;
    }
    return bs.count ? y - y0 + rowH : 0;
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
            p.hint = j < 3 ? [NSString stringWithFormat:@"%lu", (unsigned long)j + 1] : nil;
            p.ident = done ? @"" : [NSString stringWithFormat:@"notch:pick:%ld:%lu", (long)i, (unsigned long)j];
            p.primary = done && picked == (NSInteger)j;
        } else {
            p.title = @"Skip";
            p.hint = @"S";
            p.ident = done ? @"" : [NSString stringWithFormat:@"notch:skip:%ld", (long)i];
        }
        CGFloat pw = MIN([p fitWidth], w - kPad * 2);
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

// section lays out d's figures from y in a column x, w: cells, rings,
// bars, the strip, then rows. Returns the y under them.
- (CGFloat)section:(NSDictionary *)d in:(NSView *)v x:(CGFloat)x y:(CGFloat)y width:(CGFloat)w {
    NSArray *cells = aval(d, @"cells");
    if (cells.count) {
        Cells *c = [[Cells alloc] initWithFrame:NSMakeRect(x, y, w, 56)];
        c.cells = cells;
        [v addSubview:c];
        y += 56 + 12;
    }
    NSArray *rings = aval(d, @"rings");
    if (rings.count) {
        CGFloat h = [Rings heightFor:rings];
        Rings *g = [[Rings alloc] initWithFrame:NSMakeRect(x, y, w, h)];
        g.rings = rings;
        [v addSubview:g];
        y += h + 8;
    }
    NSArray *bars = aval(d, @"bars");
    if (bars.count) {
        CGFloat h = [Bars heightFor:bars];
        Bars *b = [[Bars alloc] initWithFrame:NSMakeRect(x, y, w, h)];
        b.bars = bars;
        [v addSubview:b];
        y += h + 10;
    }
    NSArray *strip = aval(d, @"strip");
    if (strip.count) {
        Strip *st = [[Strip alloc] initWithFrame:NSMakeRect(x, y + 2, w, 26)];
        st.kinds = strip;
        st.axis = aval(d, @"axis");
        [v addSubview:st];
        y += 30 + 8;
    }
    NSArray *rows = aval(d, @"rows");
    if (rows.count) {
        Rows *r = [[Rows alloc] initWithFrame:NSMakeRect(x, y, w, rows.count * 21)];
        r.rows = rows;
        [v addSubview:r];
        y += rows.count * 21 + 6;
    }
    return y;
}

// pageView lays out one page for size level lv: the page, its Mid from M
// on, its Big in a second column at L.
- (NSView *)pageView:(NSDictionary *)pg level:(NSInteger)lv width:(CGFloat)w height:(CGFloat *)outH {
    FlipView *c = [[FlipView alloc] initWithFrame:NSMakeRect(0, 0, w, 10)];
    CGFloat inner = w - kPad * 2;
    NSDictionary *big = lv >= 2 ? dval(pg, @"big") : nil;
    CGFloat colW = big ? floor(inner * 0.6) : inner;
    NSFont *lf = sys(10.5, NSFontWeightSemibold);
    NSTextField *t = text(sval(pg, @"title").uppercaseString, lf, white(0.45));
    t.frame = NSMakeRect(kPad, 0, colW, t.frame.size.height);
    [c addSubview:t];
    CGFloat y = t.frame.size.height + 6, top = y;
    NSString *lead = sval(pg, @"lead"), *note = sval(pg, @"note");
    if (lead.length) {
        NSTextField *l = text(lead, sys(14, NSFontWeightSemibold), white(1));
        l.frame = NSMakeRect(kPad, y, colW, l.frame.size.height);
        l.lineBreakMode = NSLineBreakByTruncatingTail;
        [c addSubview:l];
        y += l.frame.size.height + 8;
    }
    if (note.length) {
        NSTextField *n = wrapped(note, sys(13, NSFontWeightRegular), white(0.62), colW, 2);
        n.frame = NSMakeRect(kPad, y, colW, n.frame.size.height);
        [c addSubview:n];
        y += n.frame.size.height + 10;
    }
    y = [self section:pg in:c x:kPad y:y width:colW];
    NSDictionary *mid = lv >= 1 ? dval(pg, @"mid") : nil;
    if (mid) {
        y = [self section:mid in:c x:kPad y:y width:colW];
    }
    if (big) {
        CGFloat bx = kPad + colW + 20, bw = inner - colW - 20;
        if (sval(big, @"title").length) {
            NSTextField *bt = text(sval(big, @"title").uppercaseString, lf, white(0.45));
            bt.frame = NSMakeRect(bx, 0, bw, bt.frame.size.height);
            [c addSubview:bt];
        }
        y = MAX(y, [self section:big in:c x:bx y:top width:bw]);
    }
    c.frame = NSMakeRect(0, 0, w, y);
    *outH = y;
    return c;
}

// pagesPanel is the open quiet panel with pages: the face, the pages side
// by side in a track you swipe, their dots, then the buttons. It is as wide
// as its size and as tall as its tallest page.
- (NSView *)pagesPanel:(NSDictionary *)p size:(NSSize *)out {
    NSArray *pages = aval(p, @"pages");
    NSInteger lv = [self level];
    CGFloat w = [self levelWidth:lv];
    FlipView *v = [[FlipView alloc] initWithFrame:NSMakeRect(0, 0, w, 100)];
    [v addSubview:[self wingsWidth:w face:dval(p, @"face")]];
    CGFloat y = self.notchH + kGap + 4;
    // the same page stays in view when the pages are read again
    NSMutableArray<NSString *> *ids = [NSMutableArray new];
    for (NSDictionary *pg in pages) {
        [ids addObject:sval(pg, @"id")];
    }
    NSString *cur = self.pageAt < (NSInteger)self.pageIDs.count ? self.pageIDs[self.pageAt]
                                                                : [[NSUserDefaults standardUserDefaults] stringForKey:kPageKey];
    NSUInteger k = cur ? [ids indexOfObject:cur] : NSNotFound;
    self.pageAt = k == NSNotFound ? 0 : (NSInteger)k;
    self.pageIDs = ids;
    self.pageCount = (NSInteger)pages.count;
    FlipView *clip = [[FlipView alloc] initWithFrame:NSMakeRect(0, y, w, 10)];
    clip.wantsLayer = YES;
    clip.layer.masksToBounds = YES;
    FlipView *track = [[FlipView alloc] initWithFrame:NSZeroRect];
    CGFloat ch = 0;
    for (NSUInteger i = 0; i < pages.count; i++) {
        CGFloat h = 0;
        NSView *pv = [self pageView:pages[i] level:lv width:w height:&h];
        pv.frame = NSMakeRect(i * w, 0, w, h);
        [track addSubview:pv];
        ch = MAX(ch, h);
    }
    track.frame = NSMakeRect(0, 0, w * pages.count, ch);
    clip.frame = NSMakeRect(0, y, w, ch);
    [clip addSubview:track];
    [v addSubview:clip];
    self.track = track;
    y += ch + 4;
    PageDots *d = [[PageDots alloc] initWithFrame:NSMakeRect(0, y, w, 8)];
    d.count = (NSInteger)pages.count;
    d.at = self.pageAt;
    [v addSubview:d];
    self.dots = d;
    y += 8 + 12;
    CGFloat bh = [self addButtons:aval(p, @"buttons") to:v y:y width:w];
    y += bh ? bh + 16 : 4;
    y = MIN(y, MIN(kMaxH - kSlop, MAX(self.screenFrame.size.height * 0.45, 240)));
    v.frame = NSMakeRect(0, 0, w, y);
    *out = NSMakeSize(w, y);
    return v;
}

// panelView lays out the open panel for state; returns it with its size.
- (NSView *)panelView:(NSDictionary *)p state:(NSString *)state size:(NSSize *)out {
    if (aval(p, @"pages").count && !self.momentHere) {
        return [self pagesPanel:p size:out];
    }
    CGFloat w = [self panelWidth:state];
    FlipView *v = [[FlipView alloc] initWithFrame:NSMakeRect(0, 0, w, 100)];
    [v addSubview:[self wingsWidth:w face:dval(p, @"face")]];
    CGFloat y = self.notchH + kGap;
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
    y = MIN(y, kMaxH - kSlop);
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
    objc_setAssociatedObject(v, "size", [NSValue valueWithSize:sz], OBJC_ASSOCIATION_RETAIN);
    v.alphaValue = fade ? 0 : 1;
    [self addSubview:v];
    if (fade && !reduceMotion) {
        springJump(&CO, 0);
        springJump(&CY, -6);
        __weak Island *me = self;
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.09 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
            Island *s = me;
            if (s && s.content == v) {
                springTo(&s->CO, 1, kFade);
                springTo(&s->CY, 0, kFade);
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

// aim points the springs at the shape of what shows.
- (void)aimWidth:(CGFloat)w height:(CGFloat)h radius:(CGFloat)r alpha:(CGFloat)a params:(SpringParams)p {
    self.target = CGSizeMake(w, h);
    if (reduceMotion) {
        springJump(&W, w);
        springJump(&H, h);
        springJump(&R, r);
        springJump(&O, a);
    } else {
        springTo(&W, w, p);
        springTo(&H, h, p);
        springTo(&R, r, p);
        springTo(&O, a, kFade);
    }
    if (a > 0 && !self.win.visible) {
        [self.win orderFrontRegardless];
    }
    [self layoutWindow:NO];
    [self paint]; // new content sits where it belongs from its first frame
    kick();
}

// render shows what spec asks for on this display: the panel when it opens
// here, else the face, else the bare notch.
- (void)render:(SpringParams)p {
    [self.timers removeAllObjects];
    NSString *state = sval(spec, @"state");
    NSDictionary *pd = [self panelDict];
    NSDictionary *f = dval(spec, @"face");
    BOOL bare = !spec || faceEmpty(f);
    NSString *look = pd ? [NSString stringWithFormat:@"panel|%@|%lld|%d|%d|%lu|%ld", state, ival(spec, @"notice"), self.momentHere,
                                                     bval(spec, @"pending"), (unsigned long)aval(pd, @"pages").count, (long)[self level]]
                        : [NSString stringWithFormat:@"face|%@|%d|%d", spec ? state : @"-", bare, ival(f, @"end_ms") > 0 ? 1 : 0];
    BOOL fade = ![look isEqualToString:self.look ?: @""];
    self.look = look;
    BOOL wasPanel = self.panelShown;
    self.panelShown = pd != nil;
    if (pd) {
        if (fade) {
            self.trackAt = 0;
        }
        self.pageCount = 0;
        self.dots = nil;
        NSSize sz;
        NSView *v = [self panelView:pd state:state size:&sz];
        [self show:v size:sz fade:fade];
        if (self.pageCount > 0 && !hswipe) {
            springJump(&TX, self.pageAt);
        }
        if (self.resizing) { // the grip sets the shape; only the content changes
            [self layoutWindow:NO];
            [self paint];
            kick();
        } else {
            [self aimWidth:sz.width height:sz.height radius:24 alpha:1 params:p];
        }
        if (self.pageCount == 0) {
            [self placeTrack];
            [self endPageGesture];
        }
        [self aimGrips];
        return;
    }
    self.track = nil;
    self.pageCount = 0;
    self.dots = nil;
    [self endPageGesture];
    [self aimGrips];
    if (wasPanel) {
        self.engaged = NO;
        if (self.win.isKeyWindow) {
            // a click made the panel key: give the keys back to the app
            // underneath (ordering out hands key to its window)
            [self.win orderOut:nil];
            [self.win orderFrontRegardless];
        }
    }
    BOOL peek = self.peek && spec;
    if (bare) {
        [self show:[[FlipView alloc] initWithFrame:NSZeroRect] size:NSZeroSize fade:fade];
        CGFloat a = 1;
        if (!spec || (self.barHidden && !self.real && !peek)) {
            a = 0; // tock off, or a drawn notch over a full-screen app
        }
        if (self.real && !peek) { // well inside the camera: hidden, still clickable
            [self aimWidth:self.notchW - 12 height:self.notchH - 4 radius:8 alpha:a params:p];
        } else {
            [self aimWidth:self.notchW + (peek ? kPeekW : 0) height:self.notchH + (peek ? kPeekH : 0) radius:10 alpha:a params:p];
        }
        return;
    }
    CGFloat w = [self faceWidth:f];
    NSView *v = [self split:[self wingsWidth:w face:f] width:w];
    [self show:v size:NSMakeSize(w, self.notchH) fade:fade];
    [self aimWidth:w + (peek ? kPeekW * 0.6 : 0) height:self.notchH + (peek ? kPeekH * 0.7 : 0) radius:12 alpha:1 params:p];
}

// placeTrack moves the labelling cards to the shown entry: right away, or
// once the saved line of the entry just picked has been read.
- (void)placeTrack {
    if (!self.track) {
        return;
    }
    NSDictionary *p = dval(spec, @"panel");
    NSInteger at = (NSInteger)ival(p, @"at");
    NSArray *es = aval(p, @"entries");
    if (at > self.trackAt && self.trackAt < (NSInteger)es.count) {
        // stay on the entry just answered, then slide
        springJump(&TX, self.trackAt);
        BOOL saved = sval(es[self.trackAt], @"saved").length > 0;
        [self.slideTimer invalidate];
        __weak Island *me = self;
        self.slideTimer = [NSTimer scheduledTimerWithTimeInterval:saved ? 1.15 : 0.05
                                                          repeats:NO
                                                            block:^(NSTimer *t) {
                                                                (void)t;
                                                                Island *s = me;
                                                                if (s) {
                                                                    springTo(&s->TX, at, kSlide);
                                                                    kick();
                                                                }
                                                            }];
    } else {
        springJump(&TX, MIN(at, MAX((NSInteger)es.count - 1, 0)));
    }
    self.trackAt = at;
}

// place lays a content view over the shape. A face keeps its two wings on
// the edges of the shape, so its text never clips as the shape grows; a
// panel stays centred and shows as much of itself as the shape covers.
- (void)place:(NSView *)v dy:(CGFloat)dy {
    NSRect s = [self shapeRect];
    NSSize sz = [objc_getAssociatedObject(v, "size") sizeValue];
    BOOL wings = objc_getAssociatedObject(v, "wings") != nil;
    CGFloat cw = wings ? MAX(s.size.width, 0) : sz.width, ch = sz.height;
    NSRect cr = NSMakeRect(wings ? NSMinX(s) : round(NSMidX(s) - cw / 2), NSMinY(s) + dy, cw, ch);
    v.frame = cr;
    if (wings && v.subviews.count == 2) {
        NSView *r = v.subviews[1];
        v.subviews[0].frameOrigin = NSZeroPoint;
        r.frameOrigin = NSMakePoint(cw - r.frame.size.width, 0);
    }
    if (!v.layer) {
        v.wantsLayer = YES;
    }
    CALayer *mask = v.layer.mask;
    if (!mask) {
        mask = [CALayer layer];
        mask.backgroundColor = NSColor.blackColor.CGColor;
        v.layer.mask = mask;
    }
    // layer coordinates are not flipped: keep the top part of the content
    CGFloat vis = MAX(0, NSMaxY(s) - NSMinY(cr));
    CGFloat mx = MAX(0, NSMinX(s) - NSMinX(cr));
    mask.frame = CGRectMake(mx, MAX(0, ch - vis), MIN(cw, s.size.width), MIN(vis, ch));
}

- (void)paint {
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    if (self.content) {
        [self place:self.content dy:CY.x];
        self.content.alphaValue = MIN(MAX(CO.x, 0), 1) * MIN(MAX(O.x, 0), 1);
    }
    for (NSView *l in self.leaving) {
        [self place:l dy:0];
        l.alphaValue = leavingAlpha * MIN(MAX(O.x, 0), 1);
    }
    if (self.track) {
        self.track.frameOrigin = NSMakePoint(round(-TX.x * self.contentSize.width), 0);
    }
    if (self.dots) {
        NSInteger at = lround(MIN(MAX(TX.x, 0), MAX(self.pageCount - 1, 0)));
        if (at != self.dots.at) {
            self.dots.at = at;
            self.dots.needsDisplay = YES;
        }
    }
    [CATransaction commit];
    self.needsDisplay = YES;
}

// split turns a face laid out w wide into its two wings, which place keeps
// on the edges of the shape.
- (NSView *)split:(NSView *)v width:(CGFloat)w {
    CGFloat h = v.frame.size.height, half = floor(w / 2);
    FlipView *box = [[FlipView alloc] initWithFrame:v.frame];
    FlipView *l = [[FlipView alloc] initWithFrame:NSMakeRect(0, 0, half, h)];
    FlipView *r = [[FlipView alloc] initWithFrame:NSMakeRect(half, 0, w - half, h)];
    for (NSView *x in [v.subviews copy]) {
        if (NSMidX(x.frame) < half) {
            [l addSubview:x];
        } else {
            NSRect f = x.frame;
            f.origin.x -= half;
            [r addSubview:x];
            x.frame = f;
        }
    }
    [box addSubview:l];
    [box addSubview:r];
    objc_setAssociatedObject(box, "wings", @YES, OBJC_ASSOCIATION_RETAIN);
    return box;
}

// layoutWindow sizes the window to the island: while it moves, to cover
// both where it is and where it goes; at rest, to the island only, so the
// rest of the menu bar and the screen keep their clicks.
- (void)layoutWindow:(BOOL)settle {
    CGFloat w = settle ? MAX(W.x, 0) : MAX(MAX(W.x, 0), self.target.width) + kSlop;
    CGFloat h = settle ? MAX(H.x, 0) : MAX(MAX(H.x, 0), self.target.height) + kSlop;
    w = MIN(w, kMaxW) + kEar * 2;
    h = MIN(MAX(h, 1), kMaxH);
    NSRect f = NSMakeRect(round(self.midX - w / 2), self.top - ceil(h), ceil(w), ceil(h));
    if (!NSEqualRects(f, self.win.frame)) {
        [self.win setFrame:f display:NO];
    }
    self.frame = NSMakeRect(0, 0, f.size.width, f.size.height);
}

- (BOOL)atRest {
    Spring *ss[] = {&W, &H, &R, &O, &CO, &CY, &TX, &G};
    for (size_t i = 0; i < sizeof ss / sizeof ss[0]; i++) {
        if (!springRest(ss[i])) {
            return NO;
        }
    }
    return self.leaving.count == 0;
}

- (void)step:(double)dt {
    if ([self atRest]) {
        return;
    }
    Spring *ss[] = {&W, &H, &R, &O, &CO, &CY, &TX, &G};
    for (int k = 0; k < 4; k++) {
        for (size_t i = 0; i < sizeof ss / sizeof ss[0]; i++) {
            springStep(ss[i], dt / 4);
        }
    }
    if (self.leaving.count) {
        leavingAlpha -= dt / kFadeOut.response;
        if (leavingAlpha <= 0) {
            leavingAlpha = 0;
            for (NSView *v in self.leaving) {
                [v removeFromSuperview];
            }
            [self.leaving removeAllObjects];
        }
    }
    [self paint];
    if ([self atRest]) {
        [self layoutWindow:YES];
        [self paint];
        if (O.x <= 0) {
            [self.win orderOut:nil];
        }
    }
}

- (void)teardown {
    if (self.userOpen && !bval(spec, @"pending")) {
        notchSend(@"notch:shut"); // the display went away with the panel open
    }
    for (NSTimer *t in @[ self.intentTimer ?: [NSNull null], self.leaveTimer ?: [NSNull null], self.slideTimer ?: [NSNull null] ]) {
        if ([t isKindOfClass:[NSTimer class]]) {
            [t invalidate];
        }
    }
    [self.win orderOut:nil];
    self.win.contentView = nil;
    self.win = nil;
}

@end

// pillDone folds a panel you opened once one of its buttons did its job;
// a moment's buttons are answered by Go, which closes the moment.
static void pillDone(NSWindow *w) {
    for (Island *i in islands) {
        if (i.win == w && i.userOpen) {
            [i closeUser:kClose];
        }
    }
}

// ---------- displays ----------

// menuBars reads where each display's menu bar is: a Window Server window
// at the main menu level along the top of the display (CG coordinates, top
// left of the first display). A display in full screen has none.
static NSArray<NSValue *> *menuBars(void) {
    NSMutableArray *out = [NSMutableArray new];
    CFArrayRef ws = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
    if (!ws) {
        return out;
    }
    for (NSDictionary *w in (__bridge NSArray *)ws) {
        if ([w[(id)kCGWindowLayer] intValue] != kCGMainMenuWindowLevel || ![w[(id)kCGWindowOwnerName] isEqual:@"Window Server"]) {
            continue;
        }
        CGRect b;
        if (CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)w[(id)kCGWindowBounds], &b)) {
            [out addObject:[NSValue valueWithRect:NSRectFromCGRect(b)]];
        }
    }
    CFRelease(ws);
    return out;
}

// refreshBars finds which displays show their menu bar, and the height of
// each so a drawn notch is exactly as tall.
static void refreshBars(void) {
    NSArray<NSScreen *> *ss = [NSScreen screens];
    if (!ss.count) {
        return;
    }
    CGFloat baseTop = NSMaxY(ss[0].frame);
    NSArray<NSValue *> *bars = menuBars();
    BOOL changed = NO;
    for (Island *i in islands) {
        NSRect f = i.screenFrame;
        CGFloat cgTop = baseTop - NSMaxY(f);
        BOOL found = NO;
        CGFloat h = 0;
        for (NSValue *v in bars) {
            NSRect b = v.rectValue;
            if (fabs(NSMinY(b) - cgTop) < 2 && NSMinX(b) < NSMaxX(f) && NSMaxX(b) > NSMinX(f)) {
                found = YES;
                h = b.size.height;
            }
        }
        BOOL hide = !found && bars.count > 0; // no reading at all: assume shown
        if (hide != i.barHidden) {
            i.barHidden = hide;
            changed = YES;
        }
        if (!i.real && found && h >= 18 && h <= 44 && fabs(h - i.notchH) > 0.5) {
            i.notchH = h;
            i.look = nil;
            changed = YES;
        }
    }
    if (changed) {
        renderAll(kUser);
    }
}

// refreshBarsSoon reads the menu bars again as a space or full-screen
// change settles.
static void refreshBarsSoon(void) {
    refreshBars();
    for (NSNumber *d in @[ @0.45, @1.2 ]) {
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(d.doubleValue * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
            refreshBars();
        });
    }
}

static Island *islandAt(NSPoint p) {
    for (Island *i in islands) {
        if (NSPointInRect(p, i.screenFrame)) {
            return i;
        }
    }
    return nil;
}

// homeIsland is where a moment opens: the display under the pointer, else
// the one with the camera, else the first.
static Island *homeIsland(void) {
    for (Island *i in islands) {
        if (i.userOpen) {
            return i;
        }
    }
    Island *i = islandAt([NSEvent mouseLocation]);
    if (i) {
        return i;
    }
    for (Island *j in islands) {
        if (j.real) {
            return j;
        }
    }
    return islands.firstObject;
}

static NotchPanel *makeWindow(void) {
    NotchPanel *w = [[NotchPanel alloc] initWithContentRect:NSMakeRect(0, 0, 10, 10)
                                                  styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
                                                    backing:NSBackingStoreBuffered
                                                      defer:NO];
    w.floatingPanel = YES; // before the level: it sets the floating level
    w.level = NSMainMenuWindowLevel + 2;
    w.opaque = NO;
    w.backgroundColor = [NSColor clearColor];
    w.hasShadow = NO;
    w.hidesOnDeactivate = NO;
    w.becomesKeyOnlyIfNeeded = NO;
    w.movable = NO;
    w.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
    w.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary |
                           NSWindowCollectionBehaviorFullScreenAuxiliary | NSWindowCollectionBehaviorIgnoresCycle;
    w.sharingType = optSharing ? NSWindowSharingReadOnly : NSWindowSharingNone;
    return w;
}

// rebuild makes one island per display (or only the camera's display, else
// the main one, without [notch] all_displays).
static void rebuild(void) {
    for (Island *i in islands) {
        [i teardown];
    }
    islands = [NSMutableArray new];
    active = nil;
    NSArray<NSScreen *> *ss = [NSScreen screens];
    CGFloat cameraW = 0;
    NSScreen *camera = nil;
    for (NSScreen *s in ss) {
        if (@available(macOS 12.0, *)) {
            NSRect l = s.auxiliaryTopLeftArea, r = s.auxiliaryTopRightArea;
            CGFloat w = s.frame.size.width - l.size.width - r.size.width;
            if (s.safeAreaInsets.top > 0 && l.size.width > 0 && r.size.width > 0 && w > 0) {
                cameraW = w;
                camera = s;
                break;
            }
        }
    }
    for (NSScreen *s in ss) {
        if (!optAll && s != (camera ? camera : [NSScreen mainScreen])) {
            continue;
        }
        NSRect f = s.frame;
        Island *i = [[Island alloc] initWithFrame:NSMakeRect(0, 0, 10, 10)];
        i.screenFrame = f;
        i.displayID = [s.deviceDescription[@"NSScreenNumber"] unsignedIntValue];
        i.top = NSMaxY(f);
        i.real = s == camera;
        if (i.real) {
            if (@available(macOS 12.0, *)) {
                i.notchW = cameraW;
                i.midX = NSMaxX(s.auxiliaryTopLeftArea) + cameraW / 2;
                i.notchH = s.safeAreaInsets.top;
            }
        } else {
            i.notchW = cameraW > 0 ? cameraW : kDrawnW;
            i.midX = NSMidX(f);
            CGFloat bar = NSMaxY(f) - NSMaxY(s.visibleFrame);
            i.notchH = bar >= 18 && bar <= 44 ? bar : 24;
        }
        i.win = makeWindow();
        i.win.contentView = i;
        springJump(&i->W, i.real ? i.notchW - 12 : i.notchW);
        springJump(&i->H, i.real ? i.notchH - 4 : i.notchH);
        springJump(&i->R, 10);
        springJump(&i->O, 0);
        springJump(&i->CO, 1);
        [i layoutWindow:YES];
        [islands addObject:i];
    }
    active = homeIsland();
    refreshBars();
    renderAll(kUser);
}

// ---------- frame loop and clocks ----------

static void frame(void) {
    CFTimeInterval now = CACurrentMediaTime();
    double dt = MIN(0.05, now - lastFrame);
    lastFrame = now;
    BOOL moving = NO;
    for (Island *i in islands) {
        [i step:dt];
        moving = moving || ![i atRest];
    }
    if (!moving) {
        [frameTimer invalidate];
        frameTimer = nil;
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
    NSUInteger n = 0;
    for (Island *i in islands) {
        for (NSTextField *t in i.timers) {
            NSNumber *end = objc_getAssociatedObject(t, "end");
            t.stringValue = countdown(end.longLongValue);
            n++;
        }
    }
    if (n == 0) {
        [clockTimer invalidate];
        clockTimer = nil;
    }
}

static void startClocks(void) {
    if (clockTimer) {
        return;
    }
    clockTimer = [NSTimer timerWithTimeInterval:1 repeats:YES block:^(NSTimer *t) {
        (void)t;
        tickClocks();
    }];
    [[NSRunLoop mainRunLoop] addTimer:clockTimer forMode:NSRunLoopCommonModes];
}

static void renderAll(SpringParams p) {
    for (Island *i in islands) {
        [i render:p];
    }
    watchClicks();
    startClocks();
    tickClocks();
}

// watchClicks puts an open panel away on a click in another app. A moment
// nobody looked at folds as if its time were up, so it can wait as a dot.
static void watchClicks(void) {
    BOOL on = NO;
    for (Island *i in islands) {
        on = on || i.panelShown;
    }
    if (on && !clickMonitor) {
        clickMonitor = [NSEvent addGlobalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown | NSEventMaskRightMouseDown
                                                              handler:^(NSEvent *e) {
                                                                  (void)e;
                                                                  for (Island *i in [islands copy]) {
                                                                      if (!i.panelShown) {
                                                                          continue;
                                                                      }
                                                                      if (i.momentHere && !i.engaged && ival(spec, @"notice") > 0) {
                                                                          notchSend([NSString stringWithFormat:@"notch:expire:%lld", ival(spec, @"notice")]);
                                                                      } else {
                                                                          [i dismiss];
                                                                      }
                                                                  }
                                                              }];
    } else if (!on && clickMonitor) {
        [NSEvent removeMonitor:clickMonitor];
        clickMonitor = nil;
    }
}

// armAuto folds an open moment when its time is up, unless it is pointed
// at: then the pointer leaving puts it away.
static void armAuto(void) {
    [autoTimer invalidate];
    autoTimer = nil;
    long long secs = ival(spec, @"auto_close");
    if (!momentOpen() || secs <= 0) {
        return;
    }
    long long nid = ival(spec, @"notice");
    autoTimer = [NSTimer scheduledTimerWithTimeInterval:secs repeats:NO block:^(NSTimer *t) {
        (void)t;
        autoTimer = nil;
        if (active.inside || active.engaged) {
            return;
        }
        notchSend([NSString stringWithFormat:@"notch:expire:%lld", nid]);
    }];
}

// ---------- Go's side ----------

static void apply(NSString *s) {
    if (!islands) {
        return;
    }
    NSDictionary *d = nil;
    if (s.length) {
        d = [NSJSONSerialization JSONObjectWithData:[s dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
        if (![d isKindOfClass:[NSDictionary class]]) {
            return;
        }
    }
    NSDictionary *old = spec;
    BOOL wasOpen = bval(old, @"open"), open = bval(d, @"open");
    BOOL fresh = open && (!wasOpen || ival(old, @"notice") != ival(d, @"notice") || ![sval(old, @"state") isEqualToString:sval(d, @"state")]);
    spec = d;
    if (!d) {
        for (Island *i in islands) {
            i.userOpen = i.peek = NO;
        }
    }
    if (fresh) {
        // a new moment opens where you are, and replaces a panel you opened
        Island *home = homeIsland();
        for (Island *i in islands) {
            i.userOpen = i.byHover = i.engaged = NO;
        }
        active = home;
        active.engaged = active.inside && active.intent;
    }
    NSString *hk = sval(d, @"haptic");
    if (hk.length) {
        haptic([hk isEqualToString:@"alignment"] ? NSHapticFeedbackPatternAlignment : NSHapticFeedbackPatternLevelChange);
    }
    armAuto();
    renderAll(fresh ? kEvent : (wasOpen && !open ? kClose : kUser));
}

static void setup(void) {
    reduceMotion = [[NSWorkspace sharedWorkspace] accessibilityDisplayShouldReduceMotion];
    rebuild();
    NSNotificationCenter *nc = [NSNotificationCenter defaultCenter];
    [nc addObserverForName:NSApplicationDidChangeScreenParametersNotification
                    object:nil
                     queue:[NSOperationQueue mainQueue]
                usingBlock:^(NSNotification *n) {
                    (void)n;
                    rebuild();
                }];
    NSNotificationCenter *wc = [[NSWorkspace sharedWorkspace] notificationCenter];
    [wc addObserverForName:NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification
                    object:nil
                     queue:[NSOperationQueue mainQueue]
                usingBlock:^(NSNotification *n) {
                    (void)n;
                    reduceMotion = [[NSWorkspace sharedWorkspace] accessibilityDisplayShouldReduceMotion];
                }];
    for (NSString *name in @[ NSWorkspaceActiveSpaceDidChangeNotification, NSWorkspaceDidActivateApplicationNotification ]) {
        [wc addObserverForName:name
                        object:nil
                         queue:[NSOperationQueue mainQueue]
                    usingBlock:^(NSNotification *n) {
                        (void)n;
                        refreshBarsSoon();
                    }];
    }
    [wc addObserverForName:NSWorkspaceDidWakeNotification
                    object:nil
                     queue:[NSOperationQueue mainQueue]
                usingBlock:^(NSNotification *n) {
                    (void)n;
                    rebuild();
                }];
}

void notch_setup(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        setup();
        goNotch(islands.count > 0);
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

void bar_notch_options(int sharing, int all, int hover) {
    dispatch_async(dispatch_get_main_queue(), ^{
        BOOL again = (all != 0) != optAll;
        optSharing = sharing != 0;
        optAll = all != 0;
        optHover = hover != 0;
        if (again) {
            rebuild();
            return;
        }
        for (Island *i in islands) {
            i.win.sharingType = optSharing ? NSWindowSharingReadOnly : NSWindowSharingNone;
        }
    });
}
