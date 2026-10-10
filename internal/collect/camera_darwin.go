//go:build darwin && cgo

package collect

/*
#cgo LDFLAGS: -framework CoreMediaIO -framework CoreFoundation
#include <CoreMediaIO/CMIOHardware.h>

// tock_camera_in_use returns 1 when any video device is running in any
// process. It reads device state only, so it needs no camera permission.
static int tock_camera_in_use(void) {
	CMIOObjectPropertyAddress devs = {
		kCMIOHardwarePropertyDevices, kCMIOObjectPropertyScopeGlobal, kCMIOObjectPropertyElementMain};
	UInt32 size = 0;
	if (CMIOObjectGetPropertyDataSize(kCMIOObjectSystemObject, &devs, 0, NULL, &size) != kCMIOHardwareNoError || size == 0) {
		return 0;
	}
	CMIOObjectID ids[64];
	if (size > sizeof(ids)) {
		size = sizeof(ids);
	}
	UInt32 used = 0;
	if (CMIOObjectGetPropertyData(kCMIOObjectSystemObject, &devs, 0, NULL, size, &used, ids) != kCMIOHardwareNoError) {
		return 0;
	}
	CMIOObjectPropertyAddress running = {
		kCMIODevicePropertyDeviceIsRunningSomewhere, kCMIOObjectPropertyScopeGlobal, kCMIOObjectPropertyElementMain};
	for (UInt32 i = 0; i < used / sizeof(CMIOObjectID); i++) {
		UInt32 on = 0, got = 0;
		if (CMIOObjectGetPropertyData(ids[i], &running, 0, NULL, sizeof(on), &got, &on) == kCMIOHardwareNoError && on) {
			return 1;
		}
	}
	return 0;
}
*/
import "C"

// CameraInUse reports whether any camera is running now, in any process.
// Errors read as false.
func CameraInUse() bool { return C.tock_camera_in_use() == 1 }
