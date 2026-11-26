// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package brightness

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// wmiClient owns the root\wmi connection on one COM thread; its methods run there and wait for the result.
type wmiClient struct {
	calls chan func()
	done  chan struct{}
	svc   *ole.IDispatch // touched only on the COM thread
}

var errWMIClosed = errors.New("brightness: WMI client closed")

func openWMI() (*wmiClient, error) {
	w := &wmiClient{calls: make(chan func()), done: make(chan struct{})}
	ready := make(chan error, 1)
	go w.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return w, nil
}

func (w *wmiClient) run(ready chan<- error) {
	runtime.LockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		ready <- err
		return
	}
	defer ole.CoUninitialize()
	svc, err := connectWMI()
	if err != nil {
		ready <- err
		return
	}
	w.svc = svc
	defer svc.Release()
	ready <- nil
	for {
		select {
		case fn := <-w.calls:
			fn()
		case <-w.done:
			return
		}
	}
}

func (w *wmiClient) do(fn func()) error {
	finished := make(chan struct{})
	select {
	case w.calls <- func() { fn(); close(finished) }:
		<-finished
		return nil
	case <-w.done:
		return errWMIClosed
	}
}

func (w *wmiClient) close() { close(w.done) }

func connectWMI() (*ole.IDispatch, error) {
	locator, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, err
	}
	defer locator.Release()
	disp, err := locator.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}
	defer disp.Release()
	svc, err := oleutil.CallMethod(disp, "ConnectServer", nil, `root\wmi`)
	if err != nil {
		return nil, err
	}
	return svc.ToIDispatch(), nil
}

// each runs fn for every object a WQL query returns, on the COM thread.
func (w *wmiClient) each(query string, fn func(obj *ole.IDispatch) error) error {
	result, err := oleutil.CallMethod(w.svc, "ExecQuery", query)
	if err != nil {
		return err
	}
	set := result.ToIDispatch()
	defer set.Release()
	return oleutil.ForEach(set, func(v *ole.VARIANT) error {
		obj := v.ToIDispatch()
		defer obj.Release()
		return fn(obj)
	})
}

// names returns the EDID names of the monitors, keyed as WMIKey returns them.
func (w *wmiClient) names() (map[string]string, error) {
	names := map[string]string{}
	var queryErr error
	err := w.do(func() {
		queryErr = w.each("SELECT InstanceName, UserFriendlyName FROM WmiMonitorID", func(obj *ole.IDispatch) error {
			instance, err := oleutil.GetProperty(obj, "InstanceName")
			if err != nil {
				return err
			}
			friendly, err := oleutil.GetProperty(obj, "UserFriendlyName")
			if err != nil {
				return err
			}
			if name := edidString(friendly); name != "" {
				names[WMIInstanceKey(instance.ToString())] = name
			}
			return nil
		})
	})
	return names, errors.Join(err, queryErr)
}

// edidString decodes WMI's uint16 array of characters, padded with zeros.
func edidString(v *ole.VARIANT) string {
	arr := v.ToArray()
	if arr == nil {
		return ""
	}
	var b []rune
	for _, c := range arr.ToValueArray() {
		var r rune
		switch n := c.(type) {
		case int32:
			r = n
		case uint16:
			r = rune(n)
		case int64:
			r = rune(n)
		}
		if r == 0 {
			break
		}
		b = append(b, r)
	}
	return string(b)
}

// levels returns the brightness of every monitor that WMI can change, in percent, keyed as WMIKey returns them.
func (w *wmiClient) levels() (map[string]int, error) {
	levels := map[string]int{}
	var queryErr error
	err := w.do(func() {
		queryErr = w.each("SELECT InstanceName, CurrentBrightness FROM WmiMonitorBrightness", func(obj *ole.IDispatch) error {
			instance, err := oleutil.GetProperty(obj, "InstanceName")
			if err != nil {
				return err
			}
			current, err := oleutil.GetProperty(obj, "CurrentBrightness")
			if err != nil {
				return err
			}
			levels[WMIInstanceKey(instance.ToString())] = int(current.Val)
			return nil
		})
	})
	return levels, errors.Join(err, queryErr)
}

// set changes the brightness of the monitor with the given WMI key.
func (w *wmiClient) set(key string, percent int) error {
	var callErr error
	found := false
	err := w.do(func() {
		callErr = w.each("SELECT * FROM WmiMonitorBrightnessMethods", func(obj *ole.IDispatch) error {
			instance, err := oleutil.GetProperty(obj, "InstanceName")
			if err != nil {
				return err
			}
			if WMIInstanceKey(instance.ToString()) != key {
				return nil
			}
			found = true
			// SWbemObject's IDispatch rejects VT_UI1 and VT_UI4 as a type mismatch; it takes VT_I4.
			_, err = oleutil.CallMethod(obj, "WmiSetBrightness", int32(0), int32(percent))
			return err
		})
	})
	if err := errors.Join(err, callErr); err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("brightness: no WMI brightness control for %s", key)
	}
	return nil
}
