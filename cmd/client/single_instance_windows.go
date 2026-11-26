//go:build windows

package main

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sys/windows"
)

const (
	singleInstanceMutexName = `Local\TLAN-Desktop`
	singleInstanceEventName = `Local\TLAN-Desktop-Activate`
)

func acquireSingleInstanceNamed(mutexName, eventName string) (mutexHandle, activateEvent windows.Handle, alreadyRunning bool, err error) {
	evPtr := windows.StringToUTF16Ptr(eventName)
	activateEvent, evErr := windows.CreateEvent(nil, 0, 0, evPtr)
	if evErr != nil && !errors.Is(evErr, windows.ERROR_ALREADY_EXISTS) {
		return 0, 0, false, evErr
	}

	muxPtr := windows.StringToUTF16Ptr(mutexName)
	mutexHandle, muxErr := windows.CreateMutex(nil, false, muxPtr)
	if errors.Is(muxErr, windows.ERROR_ALREADY_EXISTS) || errors.Is(muxErr, windows.ERROR_ACCESS_DENIED) {
		if mutexHandle != 0 {
			_ = windows.CloseHandle(mutexHandle)
			mutexHandle = 0
		}
		if activateEvent != 0 {
			_ = windows.CloseHandle(activateEvent)
			activateEvent = 0
		}
		return 0, 0, true, nil
	}
	if muxErr != nil {
		if mutexHandle != 0 {
			_ = windows.CloseHandle(mutexHandle)
		}
		if activateEvent != 0 {
			_ = windows.CloseHandle(activateEvent)
		}
		return 0, 0, false, muxErr
	}
	return mutexHandle, activateEvent, false, nil
}

func signalSingleInstanceEvent(eventName string) bool {
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, windows.StringToUTF16Ptr(eventName))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(ev)
	return windows.SetEvent(ev) == nil
}

func watchActivateEvent(ctx context.Context, ev windows.Handle, onActivate func()) {
	if ev == 0 || onActivate == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		res, err := windows.WaitForSingleObject(ev, uint32((250*time.Millisecond)/time.Millisecond))
		if err != nil {
			return
		}
		if res == windows.WAIT_OBJECT_0 {
			onActivate()
		}
	}
}
