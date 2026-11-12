package adb

import (
	"fmt"
	"strconv"
	"strings"
)

// Android key codes used for TV control.
const (
	KeyPower   = 26  // KEYCODE_POWER (toggle)
	KeySleep   = 223 // KEYCODE_SLEEP
	KeyWakeup  = 224 // KEYCODE_WAKEUP
	KeyHome    = 3
	KeyBack    = 4
	KeyHDMI1   = 243 // KEYCODE_TV_INPUT_HDMI_1
	KeyHDMI2   = 244
	KeyHDMI3   = 245
	KeyHDMI4   = 246
	KeyVolUp   = 24
	KeyVolDown = 25
	KeyMute    = 164
)

// Command resolves a symbolic command name to an Android key code.
// Accepts: on/poweron, off/poweroff, power/toggle, hdmi1..hdmi4,
// home, back, volup/voldown, mute, or key:<code> / a bare number.
func Command(name string) (int, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "on", "poweron", "wake", "wakeup":
		return KeyWakeup, nil
	case "off", "poweroff", "sleep":
		return KeySleep, nil
	case "power", "toggle":
		return KeyPower, nil
	case "hdmi1", "hdmi_1", "input1":
		return KeyHDMI1, nil
	case "hdmi2", "hdmi_2", "input2":
		return KeyHDMI2, nil
	case "hdmi3", "hdmi_3", "input3":
		return KeyHDMI3, nil
	case "hdmi4", "hdmi_4", "input4":
		return KeyHDMI4, nil
	case "home":
		return KeyHome, nil
	case "back":
		return KeyBack, nil
	case "volup", "volumeup":
		return KeyVolUp, nil
	case "voldown", "volumedown":
		return KeyVolDown, nil
	case "mute":
		return KeyMute, nil
	}
	if strings.HasPrefix(n, "key:") {
		n = n[len("key:"):]
	}
	if code, err := strconv.Atoi(n); err == nil && code >= 0 && code < 1000 {
		return code, nil
	}
	return 0, fmt.Errorf("unknown command %q", name)
}

// SendKey injects a single key event.
func (c *Client) SendKey(code int) error {
	_, err := c.Shell(fmt.Sprintf("input keyevent %d", code))
	return err
}

// Do resolves a symbolic command and sends it.
func (c *Client) Do(command string) error {
	code, err := Command(command)
	if err != nil {
		return err
	}
	return c.SendKey(code)
}
