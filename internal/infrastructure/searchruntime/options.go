package searchruntime

import "errors"

type Options struct {
	Corpus     string
	LiveQuery  bool
	ParserMode string
}

func (o Options) Validate() error {
	if o.ParserMode != "off" && o.ParserMode != "replay" && o.ParserMode != "live" {
		return errors.New("parser-mode must be off, replay or live")
	}
	if o.Corpus == "" && (o.LiveQuery || o.ParserMode != "off") {
		return errors.New("live-query and parser require vector-corpus")
	}
	if o.ParserMode == "live" && !o.LiveQuery {
		return errors.New("live parser requires live-query")
	}
	return nil
}
