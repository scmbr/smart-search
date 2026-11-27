package main

import "errors"

func validateParserMode(mode, corpus string, liveQuery, batch bool) error {
	if mode != "off" && mode != "replay" && mode != "live" {
		return errors.New("parser-mode must be off, replay or live")
	}
	if mode != "off" && corpus == "" {
		return errors.New("parser requires vector-corpus")
	}
	if mode == "live" && (!liveQuery || batch) {
		return errors.New("live parser requires live-query and a single query (no batch)")
	}
	return nil
}
