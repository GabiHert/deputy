#import <Cocoa/Cocoa.h>
#include <stdatomic.h>
#include "menubar_darwin.h"

// Implemented in Go; the numbers match the action constants in menubar.go.
extern void positronMenuBarAction(int action);

@interface PositronMenuTarget : NSObject
- (void)pick:(NSMenuItem *)sender;
@end

@implementation PositronMenuTarget
- (void)pick:(NSMenuItem *)sender {
	positronMenuBarAction((int)sender.tag);
}
@end

static NSStatusItem *statusItem;
static PositronMenuTarget *target;
static int waiting;
static int unread;
static atomic_int positronFocused; // 1 while one of Positron's windows has the keyboard focus

static NSMenuItem *menuItem(NSString *title, NSInteger tag) {
	NSMenuItem *mi = [[NSMenuItem alloc] initWithTitle:title action:@selector(pick:) keyEquivalent:@""];
	mi.target = target;
	mi.tag = tag;
	return mi;
}

// logoImage draws Positron's mark (a chat bubble with typing dots on a tilted orbit, with two
// particles) as a template image, from the same 64x64 drawing as the app icon. With replies
// waiting, the dots become one filled dot so the icon reads differently at a glance.
static NSImage *logoImage(BOOL attention) {
	NSImage *image = [NSImage imageWithSize:NSMakeSize(18, 18) flipped:YES drawingHandler:^BOOL(NSRect rect) {
		NSAffineTransform *t = [NSAffineTransform transform];
		[t scaleBy:18.0 / 64.0];
		[t concat];
		[[NSColor blackColor] set];

		// The bubble.
		NSBezierPath *b = [NSBezierPath bezierPath];
		[b moveToPoint:NSMakePoint(24, 19)];
		[b appendBezierPathWithArcFromPoint:NSMakePoint(50, 19) toPoint:NSMakePoint(50, 43) radius:5];
		[b appendBezierPathWithArcFromPoint:NSMakePoint(50, 43) toPoint:NSMakePoint(14, 43) radius:5];
		[b lineToPoint:NSMakePoint(30, 43)];
		[b lineToPoint:NSMakePoint(22, 49)];
		[b lineToPoint:NSMakePoint(22, 43)];
		[b appendBezierPathWithArcFromPoint:NSMakePoint(14, 43) toPoint:NSMakePoint(14, 19) radius:5];
		[b appendBezierPathWithArcFromPoint:NSMakePoint(14, 19) toPoint:NSMakePoint(50, 19) radius:5];
		[b closePath];
		// The orbit, tilted 24 degrees at half strength, is clipped away inside the bubble.
		[NSGraphicsContext saveGraphicsState];
		NSBezierPath *outside = [NSBezierPath bezierPathWithRect:NSMakeRect(0, 0, 64, 64)];
		[outside appendBezierPath:b];
		outside.windingRule = NSWindingRuleEvenOdd;
		[outside addClip];
		NSAffineTransform *tilt = [NSAffineTransform transform];
		[tilt translateXBy:32 yBy:32];
		[tilt rotateByDegrees:-24];
		NSBezierPath *orbit = [NSBezierPath bezierPathWithOvalInRect:NSMakeRect(-27, -12, 54, 24)];
		[orbit transformUsingAffineTransform:tilt];
		orbit.lineWidth = 3.4;
		[[NSColor colorWithWhite:0 alpha:0.5] set];
		[orbit stroke];
		[NSGraphicsContext restoreGraphicsState];

		b.lineWidth = 5.5;
		b.lineJoinStyle = NSLineJoinStyleRound;
		[b stroke];

		if (attention) {
			[[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(26, 25, 12, 12)] fill];
		} else {
			for (int i = 0; i < 3; i++) {
				[[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(20 + i * 8.5, 27.5, 7, 7)] fill];
			}
		}
		// The particles on the orbit.
		[[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(56.67 - 4.6, 21.02 - 4.6, 9.2, 9.2)] fill];
		[[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(7.33 - 4.6, 42.98 - 4.6, 9.2, 9.2)] fill];
		return YES;
	}];
	image.accessibilityDescription = @"Positron";
	return image;
}

// render sets the icon and rebuilds the menu. Main thread only.
static void render(void) {
	if (statusItem == nil) {
		return;
	}
	NSImage *image = logoImage(waiting > 0);
	image.template = YES;
	statusItem.button.image = image;
	statusItem.button.title = waiting > 0 ? [NSString stringWithFormat:@" %d", waiting] : @"";
	statusItem.button.imagePosition = NSImageLeft;
	statusItem.button.toolTip = waiting > 0 ? [NSString stringWithFormat:@"Positron: %d %@ waiting", waiting, waiting == 1 ? @"reply" : @"replies"] : @"Positron";

	NSMenu *menu = [[NSMenu alloc] init];
	if (unread > 0) {
		[menu addItem:menuItem([NSString stringWithFormat:@"%d unread %@", unread, unread == 1 ? @"message" : @"messages"], 3)];
	}
	if (waiting > 0) {
		[menu addItem:menuItem([NSString stringWithFormat:@"Review %d waiting %@", waiting, waiting == 1 ? @"reply" : @"replies"], 0)];
	} else {
		[menu addItem:menuItem(@"Show activity", 0)];
	}
	[menu addItem:menuItem(@"Open Positron", 1)];
	[menu addItem:[NSMenuItem separatorItem]];
	[menu addItem:menuItem(@"Quit Positron", 2)];
	statusItem.menu = menu;
}

void PositronMenuBarInstall(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (statusItem != nil) {
			return;
		}
		target = [[PositronMenuTarget alloc] init];
		statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
		render();
		NSNotificationCenter *nc = [NSNotificationCenter defaultCenter];
		[nc addObserverForName:NSWindowDidBecomeKeyNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
			atomic_store(&positronFocused, 1);
		}];
		[nc addObserverForName:NSWindowDidResignKeyNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
			atomic_store(&positronFocused, NSApp.keyWindow != nil ? 1 : 0);
		}];
		[nc addObserverForName:NSApplicationDidResignActiveNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
			atomic_store(&positronFocused, 0);
		}];
		atomic_store(&positronFocused, (NSApp.isActive && NSApp.keyWindow != nil) ? 1 : 0);
	});
}

void PositronMenuBarSetUnread(int n) {
	dispatch_async(dispatch_get_main_queue(), ^{
		unread = n;
		NSApp.dockTile.badgeLabel = n > 0 ? (n > 999 ? @"999+" : [NSString stringWithFormat:@"%d", n]) : nil;
		render();
	});
}

int PositronAppFocused(void) {
	return atomic_load(&positronFocused);
}

void PositronMenuBarSetWaiting(int n) {
	dispatch_async(dispatch_get_main_queue(), ^{
		waiting = n;
		render();
	});
}
