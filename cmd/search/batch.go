package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"

	app "github.com/scmbr/smart-search/internal/application/lexical"
	d "github.com/scmbr/smart-search/internal/domain/search"
)

func searchBatch(ctx context.Context, backend app.Backend, template d.Request, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 4096)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var query string
		if err := json.Unmarshal(scanner.Bytes(), &query); err != nil {
			return errors.New("batch expects one JSON query string per line")
		}
		request := template
		request.Query = query
		result, err := app.Search(ctx, backend, request)
		if err != nil {
			return err
		}
		if err := encoder.Encode(result); err != nil {
			return err
		}
	}
	return scanner.Err()
}
