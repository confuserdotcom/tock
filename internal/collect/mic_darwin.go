//go:build darwin && cgo

package collect

/*
#cgo LDFLAGS: -framework CoreAudio -framework CoreFoundation
#include <CoreAudio/CoreAudio.h>

// tock_mic_in_use returns 1 when any device with input streams is running in
// any process. It reads device state only, so it needs no microphone
// permission.
static int tock_mic_in_use(void) {
	AudioObjectPropertyAddress devs = {
		kAudioHardwarePropertyDevices, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
	UInt32 size = 0;
	if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &devs, 0, NULL, &size) != noErr || size == 0) {
		return 0;
	}
	AudioObjectID ids[64];
	if (size > sizeof(ids)) {
		size = sizeof(ids);
	}
	if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &devs, 0, NULL, &size, ids) != noErr) {
		return 0;
	}
	AudioObjectPropertyAddress streams = {
		kAudioDevicePropertyStreams, kAudioObjectPropertyScopeInput, kAudioObjectPropertyElementMain};
	AudioObjectPropertyAddress running = {
		kAudioDevicePropertyDeviceIsRunningSomewhere, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
	for (UInt32 i = 0; i < size / sizeof(AudioObjectID); i++) {
		UInt32 ssize = 0;
		if (AudioObjectGetPropertyDataSize(ids[i], &streams, 0, NULL, &ssize) != noErr || ssize == 0) {
			continue; // output-only device
		}
		UInt32 on = 0, osize = sizeof(on);
		if (AudioObjectGetPropertyData(ids[i], &running, 0, NULL, &osize, &on) == noErr && on) {
			return 1;
		}
	}
	return 0;
}
*/
import "C"

// MicInUse reports whether any microphone is running now, in any process.
// Errors read as false.
func MicInUse() bool { return C.tock_mic_in_use() == 1 }
