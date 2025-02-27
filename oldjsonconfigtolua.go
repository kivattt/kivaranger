package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type oldConfig struct {
	UiBorders               bool               `json:"ui-borders"`
	NoMouse                 bool               `json:"no-mouse"`
	NoWrite                 bool               `json:"no-write"`
	DontShowHiddenFiles     bool               `json:"dont-show-hidden-files"`
	FoldersNotFirst         bool               `json:"folders-not-first"`
	PrintPathOnOpen         bool               `json:"print-path-on-open"`
	OpenWith                []openWithEntry    `json:"open-with"`
	PreviewWith             []previewWithEntry `json:"preview-with"`
	DontChangeTerminalTitle bool               `json:"dont-change-terminal-title"`
	DontShowHelpText        bool               `json:"dont-show-help-text"`
}

type openWithEntry struct {
	Programs   []string `json:"programs"`
	Match      []string `json:"match"`
	DoNotMatch []string `json:"do-not-match"`
}

type previewWithEntry struct {
	Script     string   `json:"script"`
	Programs   []string `json:"programs"`
	Match      []string `json:"match"`
	DoNotMatch []string `json:"do-not-match"`
}

func PromptForGenerateLuaConfig(configFilename string, fen *Fen) {
	if fen.config.NoWrite {
		return
	}

	fmt.Print("Generate config.lua from fenrc.json file? (This will not erase anything) [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	confirmation, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}

	if IsYes(confirmation) {
		oldConfigPath := filepath.Join(filepath.Dir(configFilename), "fenrc.json")
		newConfigPath := filepath.Join(filepath.Dir(configFilename), "config.lua")
		fmt.Print("Generate new config file: " + newConfigPath + " ? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		confirmation, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		if IsYes(confirmation) {
			err = GenerateLuaConfigFromOldJSONConfig(oldConfigPath, newConfigPath, fen)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Done! Your new config file: " + newConfigPath)
		} else {
			fmt.Println("Nothing done")
		}
	} else {
		fmt.Println("Nothing done")
	}
}

func GenerateLuaConfigFromOldJSONConfig(oldJSONConfigPath, newLuaConfigPath string, fen *Fen) error {
	bytes, err := os.ReadFile(oldJSONConfigPath)
	if err != nil {
		return err
	}

	var config oldConfig
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		return err
	}

	_, err = os.Stat(newLuaConfigPath)
	if err == nil {
		return errors.New(newLuaConfigPath + " already exists")
	}

	newConfigFile, err := os.Create(newLuaConfigPath)
	if err != nil {
		return err
	}

	newConfigFile.WriteString("-- Generated by fen " + version + " at " + time.Now().Format(time.UnixDate) + "\n")

	boolToStr := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}

	defaultConfigValues := NewConfigDefaultValues()

	// Tons of ifs to only write the required non-default boolean options
	if config.UiBorders != defaultConfigValues.UiBorders {
		newConfigFile.WriteString("fen.ui_borders = " + boolToStr(config.UiBorders) + "\n")
	}

	if config.NoWrite != defaultConfigValues.NoWrite {
		newConfigFile.WriteString("fen.no_write = " + boolToStr(config.NoWrite) + "\n")
	}

	if config.PrintPathOnOpen != defaultConfigValues.PrintPathOnOpen {
		newConfigFile.WriteString("fen.print_path_on_open = " + boolToStr(config.PrintPathOnOpen) + "\n")
	}

	if config.NoMouse == defaultConfigValues.Mouse {
		newConfigFile.WriteString("fen.mouse = " + boolToStr(!config.NoMouse) + "\n")
	}

	if config.DontShowHiddenFiles == defaultConfigValues.HiddenFiles {
		newConfigFile.WriteString("fen.hidden_files = " + boolToStr(!config.DontShowHiddenFiles) + "\n")
	}

	if config.FoldersNotFirst == defaultConfigValues.FoldersFirst {
		newConfigFile.WriteString("fen.folders_first = " + boolToStr(!config.FoldersNotFirst) + "\n")
	}

	if config.DontChangeTerminalTitle == defaultConfigValues.TerminalTitle {
		newConfigFile.WriteString("fen.terminal_title = " + boolToStr(!config.DontChangeTerminalTitle) + "\n")
	}

	if config.DontShowHelpText == defaultConfigValues.ShowHelpText {
		newConfigFile.WriteString("fen.show_help_text = " + boolToStr(!config.DontShowHelpText) + "\n")
	}

	if len(config.OpenWith) > 0 {
		newConfigFile.WriteString("\n")
		newConfigFile.WriteString("fen.open = {")
		for _, openEntry := range config.OpenWith {
			newConfigFile.WriteString("\n")
			newConfigFile.WriteString("\t{")
			if len(openEntry.Programs) > 0 {
				newConfigFile.WriteString("\n\t\tprogram = {")
				for _, program := range openEntry.Programs {
					newConfigFile.WriteString("\"" + program + "\",")
				}
				newConfigFile.WriteString("},")
			}

			if len(openEntry.Match) > 0 {
				newConfigFile.WriteString("\n\t\tmatch = {")
				for _, match := range openEntry.Match {
					newConfigFile.WriteString("\"" + match + "\",")
				}
				newConfigFile.WriteString("},")
			}

			if len(openEntry.DoNotMatch) > 0 {
				newConfigFile.WriteString("\n\t\tdo_not_match = {")
				for _, doNotMatch := range openEntry.DoNotMatch {
					newConfigFile.WriteString("\"" + doNotMatch + "\",")
				}
				newConfigFile.WriteString("},")
			}

			newConfigFile.WriteString("\n\t},")
		}
		newConfigFile.WriteString("\n}\n")
	}

	replaceFenConfigPathWithVariable := func(s string, checkAbsolute bool) string {
		if len(s) < 2 {
			return s
		}

		if strings.HasPrefix(s, "\"FEN_CONFIG_PATH/") {
			s = "fen.config_path..\"" + s[len("\"FEN_CONFIG_PATH/"):]
		} else if checkAbsolute && !filepath.IsAbs(s[1:len(s)-2]) {
			s = "fen.config_path..\"" + s[1:]
		}

		return strings.ReplaceAll(s, "FEN_CONFIG_PATH/", "\"..fen.config_path..\"")
	}

	if len(config.PreviewWith) > 0 {
		newConfigFile.WriteString("\n")
		newConfigFile.WriteString("fen.preview = {")
		for _, previewEntry := range config.PreviewWith {
			newConfigFile.WriteString("\n")
			newConfigFile.WriteString("\t{")

			if len(previewEntry.Script) > 0 {
				newConfigFile.WriteString("\n\t\tscript = " + replaceFenConfigPathWithVariable("\""+previewEntry.Script+"\"", true) + ",")
			}

			if len(previewEntry.Programs) > 0 {
				newConfigFile.WriteString("\n\t\tprogram = {")
				for _, program := range previewEntry.Programs {
					newConfigFile.WriteString(replaceFenConfigPathWithVariable("\""+program+"\"", false) + ",")
				}
				newConfigFile.WriteString("},")
			}

			if len(previewEntry.Match) > 0 {
				newConfigFile.WriteString("\n\t\tmatch = {")
				for _, match := range previewEntry.Match {
					newConfigFile.WriteString("\"" + match + "\",")
				}
				newConfigFile.WriteString("},")
			}

			if len(previewEntry.DoNotMatch) > 0 {
				newConfigFile.WriteString("\n\t\tdo_not_match = {")
				for _, doNotMatch := range previewEntry.DoNotMatch {
					newConfigFile.WriteString("\"" + doNotMatch + "\",")
				}
				newConfigFile.WriteString("},")
			}

			newConfigFile.WriteString("\n\t},")
		}
		newConfigFile.WriteString("\n}\n")
	}

	newConfigFile.Close()

	if err := fen.ReadConfig(newLuaConfigPath); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to generate a valid config.lua file")
		return err
	}

	return nil
}
