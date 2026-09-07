package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type terminalCredentialPrompter struct {
	input  *os.File
	output io.Writer
}

func newTerminalCredentialPrompter() *terminalCredentialPrompter {
	return &terminalCredentialPrompter{input: os.Stdin, output: os.Stderr}
}

func (p *terminalCredentialPrompter) IsTerminal() bool {
	return p != nil && p.input != nil && term.IsTerminal(int(p.input.Fd()))
}

func (p *terminalCredentialPrompter) ReadPATName(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !p.IsTerminal() {
		return "", errors.New("PAT input requires an interactive terminal")
	}
	if _, err := fmt.Fprint(p.output, "PAT name: "); err != nil {
		return "", err
	}
	value, err := bufio.NewReader(p.input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func (p *terminalCredentialPrompter) ReadPATSecret(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !p.IsTerminal() {
		return "", errors.New("PAT input requires an interactive terminal")
	}
	if _, err := fmt.Fprint(p.output, "PAT secret: "); err != nil {
		return "", err
	}
	value, err := term.ReadPassword(int(p.input.Fd()))
	_, newlineErr := fmt.Fprintln(p.output)
	if err != nil {
		return "", err
	}
	if newlineErr != nil {
		return "", newlineErr
	}
	return string(value), nil
}
