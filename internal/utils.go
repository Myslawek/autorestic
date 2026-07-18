package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/cupcakearmy/autorestic/internal/colors"
	"github.com/cupcakearmy/autorestic/internal/flags"
	"github.com/fatih/color"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func CheckIfCommandIsCallable(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func CheckIfResticIsCallable() bool {
	return CheckIfCommandIsCallable(flags.RESTIC_BIN)
}

type ExecuteOptions struct {
	Command string
	Envs    map[string]string
	Dir     string
	Silent  bool
}

type ColoredWriter struct {
	target io.Writer
	color  *color.Color
}

func (w ColoredWriter) Write(p []byte) (n int, err error) {
	colored := []byte(w.color.Sprint(string(p)))
	w.target.Write(colored)
	return len(p), nil
}

func ExecuteCommand(options ExecuteOptions, args ...string) (int, string, error) {
	cmd := exec.Command(options.Command, args...)
	env := os.Environ()
	for k, v := range options.Envs {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
	cmd.Dir = options.Dir

	if flags.VERBOSE {
		colors.Faint.Printf("> Executing: %s\n", cmd)
	}

	var out bytes.Buffer
	var error bytes.Buffer
	if flags.VERBOSE && !options.Silent {
		var colored ColoredWriter = ColoredWriter{
			target: os.Stdout,
			color:  colors.Faint,
		}
		mw := io.MultiWriter(colored, &out)
		cmd.Stdout = mw
	} else {
		cmd.Stdout = &out
	}
	cmd.Stderr = &error
	err := cmd.Run()
	if err != nil {
		code := -1
		if exitError, ok := err.(*exec.ExitError); ok {
			code = exitError.ExitCode()
		}
		return code, error.String(), err
	}
	return 0, out.String(), nil
}

func ExecuteResticCommand(options ExecuteOptions, args ...string) (int, string, error) {
	options.Command = flags.RESTIC_BIN
	var c = GetConfig()
	var optionsAsString = getOptions(c.Global, []string{"all"})
	args = append(optionsAsString, args...)
	return ExecuteCommand(options, args...)
}

func CopyFile(from, to string) error {
	original, err := os.Open(from)
	if err != nil {
		return nil
	}
	defer original.Close()

	new, err := os.Create(to)
	if err != nil {
		return nil
	}
	defer new.Close()

	if _, err := io.Copy(new, original); err != nil {
		return err
	}
	return nil
}

func CheckIfVolumeExists(volume string) bool {
	_, _, err := ExecuteCommand(ExecuteOptions{Command: "docker"}, "volume", "inspect", volume)
	return err == nil
}

func ArrayContains[T comparable](arr []T, needle T) bool {
	for _, item := range arr {
		if item == needle {
			return true
		}
	}
	return false
}

func OmitEmpty(settings map[string]any) map[string]any {
	jsonSettings, _ := json.Marshal(settings)
	fmt.Println(string(jsonSettings))
	sanitizedSettings := make(map[string]any)
	for key, value := range settings {
		switch casted := value.(type) {
		case map[string]any:
			innerSettings := OmitEmpty(casted)
			if len(innerSettings) > 0 {
				sanitizedSettings[key] = innerSettings
			}
		case string:
			if casted != "" {
				sanitizedSettings[key] = casted
			}
		case []any:
			if len(casted) > 0 {
				sanitizedSettings[key] = casted
			}
		case nil:
			// skip
		default:
			sanitizedSettings[key] = casted
		}
	}
	return sanitizedSettings
}

func WriteConfig() error {
	viper.WriteConfig()
	flags := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	file, err := os.OpenFile(viper.ConfigFileUsed(), flags, 0o644)
	if err != nil {
		return err
	}

	defer file.Close()
	sanitizedSettings := OmitEmpty(viper.AllSettings())
	if err := yaml.NewEncoder(file).Encode(sanitizedSettings); err != nil {
		return err
	}

	return file.Sync()
}
