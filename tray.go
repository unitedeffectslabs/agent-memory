package main

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// Forward declaration — implemented in Go via //export
void trayShowClicked(void);
void trayQuitClicked(void);

// Stores the status item so it doesn't get garbage-collected.
static NSStatusItem *statusItem = nil;

static void setupStatusItem(const void *iconData, int iconLen) {
    dispatch_async(dispatch_get_main_queue(), ^{
        statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];

        NSData *data = [NSData dataWithBytes:iconData length:iconLen];
        NSImage *img = [[NSImage alloc] initWithData:data];
        [img setSize:NSMakeSize(18, 18)];
        [img setTemplate:NO];
        statusItem.button.image = img;
        statusItem.button.toolTip = @"Agent Memory";

        NSMenu *menu = [[NSMenu alloc] init];

        NSMenuItem *showItem = [[NSMenuItem alloc] initWithTitle:@"Show"
                                                          action:@selector(showClicked:)
                                                   keyEquivalent:@""];
        showItem.target = statusItem; // just needs a non-nil target; we override below
        [menu addItem:showItem];

        [menu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *quitItem = [[NSMenuItem alloc] initWithTitle:@"Quit Agent Memory"
                                                          action:@selector(quitClicked:)
                                                   keyEquivalent:@"q"];
        quitItem.target = statusItem;
        [menu addItem:quitItem];

        statusItem.menu = menu;
    });
}

static void removeStatusItem(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusItem != nil) {
            [[NSStatusBar systemStatusBar] removeStatusItem:statusItem];
            statusItem = nil;
        }
    });
}

// We use a category on NSStatusItem to handle the menu actions.
@interface NSStatusItem (AgentMemory)
- (void)showClicked:(id)sender;
- (void)quitClicked:(id)sender;
@end

@implementation NSStatusItem (AgentMemory)
- (void)showClicked:(id)sender {
    trayShowClicked();
}
- (void)quitClicked:(id)sender {
    trayQuitClicked();
}
@end
*/
import "C"

import (
	_ "embed"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed assets/trayicon.png
var trayIcon []byte

// setupTray creates a macOS status bar item with Show / Quit menu.
// Returns a cleanup function.
func (a *App) setupTray() func() {
	ptr := unsafe.Pointer(&trayIcon[0])
	C.setupStatusItem(ptr, C.int(len(trayIcon)))
	return func() {
		C.removeStatusItem()
	}
}

//export trayShowClicked
func trayShowClicked() {
	if appInstance != nil && appInstance.ctx != nil {
		runtime.WindowShow(appInstance.ctx)
	}
}

//export trayQuitClicked
func trayQuitClicked() {
	if appInstance != nil && appInstance.ctx != nil {
		runtime.Quit(appInstance.ctx)
	}
}
