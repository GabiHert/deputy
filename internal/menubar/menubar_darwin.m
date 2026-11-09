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

// logoImage draws Positron's D monogram (a D shaped like a speech bubble, with typing dots) as
// a template image, from the same 64x64 drawing as the app icon. With replies waiting, the
// dots become one filled dot so the icon reads differently at a glance.
static NSImage *logoImage(BOOL attention) {
	NSImage *image = [NSImage imageWithSize:NSMakeSize(18, 18) flipped:YES drawingHandler:^BOOL(NSRect rect) {
		NSAffineTransform *t = [NSAffineTransform transform];
		[t scaleBy:18.0 / 64.0];
		[t concat];
		[[NSColor blackColor] set];
		NSBezierPath *d = [NSBezierPath bezierPath];
		[d moveToPoint:NSMakePoint(15, 11)];
		[d lineToPoint:NSMakePoint(32, 11)];
		[d appendBezierPathWithArcWithCenter:NSMakePoint(32, 32) radius:21 startAngle:270 endAngle:90 clockwise:NO];
		[d lineToPoint:NSMakePoint(23, 53)];
		[d lineToPoint:NSMakePoint(15, 59)];
		[d closePath];
		d.lineWidth = 5.5;
		d.lineJoinStyle = NSLineJoinStyleRound;
		[d stroke];
		if (attention) {
			[[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(25, 24, 16, 16)] fill];
		} else {
			for (int i = 0; i < 3; i++) {
				[[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(21.5 + i * 8, 28.5, 7, 7)] fill];
			}
		}
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
