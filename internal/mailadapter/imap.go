package mailadapter

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type imapClient struct {
	conn   net.Conn
	reader *bufio.Reader
	tag    int
}

func Search(opts SearchOptions) (SearchResult, error) {
	account, err := FindAccount(opts.Account)
	if err != nil {
		return SearchResult{}, err
	}
	if opts.Limit <= 0 || opts.Limit > 50 {
		opts.Limit = 10
	}
	if strings.TrimSpace(opts.Mailbox) != "" {
		account.Mailbox = strings.TrimSpace(opts.Mailbox)
	}
	client, err := dialIMAP(account)
	if err != nil {
		return SearchResult{}, err
	}
	defer client.close()
	if err := client.login(account); err != nil {
		return SearchResult{}, err
	}
	if err := client.selectMailbox(account.Mailbox); err != nil {
		return SearchResult{}, err
	}
	uids, err := client.searchUIDs(opts)
	if err != nil {
		return SearchResult{}, err
	}
	if len(uids) > opts.Limit {
		uids = uids[len(uids)-opts.Limit:]
	}
	messages := make([]MessageSummary, 0, len(uids))
	for i := len(uids) - 1; i >= 0; i-- {
		msg, fetchErr := client.fetchMessage(account.Mailbox, uids[i], 1200)
		if fetchErr == nil {
			messages = append(messages, msg.Summary)
		}
	}
	return SearchResult{
		Kind:        "meshclaw_mail_search",
		Account:     publicAccount(account),
		Query:       strings.TrimSpace(opts.Query),
		Since:       opts.Since,
		Limit:       opts.Limit,
		Messages:    messages,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

func Read(opts ReadOptions) (Message, error) {
	account, err := FindAccount(opts.Account)
	if err != nil {
		return Message{}, err
	}
	if strings.TrimSpace(opts.Mailbox) != "" {
		account.Mailbox = strings.TrimSpace(opts.Mailbox)
	}
	if strings.TrimSpace(opts.ID) == "" {
		return Message{}, errors.New("message id is required")
	}
	maxBody := opts.MaxBody
	if maxBody <= 0 || maxBody > 20000 {
		maxBody = 5000
	}
	client, err := dialIMAP(account)
	if err != nil {
		return Message{}, err
	}
	defer client.close()
	if err := client.login(account); err != nil {
		return Message{}, err
	}
	if err := client.selectMailbox(account.Mailbox); err != nil {
		return Message{}, err
	}
	return client.fetchMessage(account.Mailbox, opts.ID, maxBody)
}

func ReadMany(opts ReadManyOptions) (ReadManyResult, error) {
	account, err := FindAccount(opts.Account)
	if err != nil {
		return ReadManyResult{}, err
	}
	if strings.TrimSpace(opts.Mailbox) != "" {
		account.Mailbox = strings.TrimSpace(opts.Mailbox)
	}
	maxBody := opts.MaxBody
	if maxBody <= 0 || maxBody > 20000 {
		maxBody = 5000
	}
	ids := cleanMessageIDs(opts.IDs)
	if len(ids) == 0 {
		return ReadManyResult{}, errors.New("at least one message id is required")
	}
	if len(ids) > 20 {
		return ReadManyResult{}, errors.New("read-many supports at most 20 message ids per call")
	}
	client, err := dialIMAP(account)
	if err != nil {
		return ReadManyResult{}, err
	}
	defer client.close()
	if err := client.login(account); err != nil {
		return ReadManyResult{}, err
	}
	if err := client.selectMailbox(account.Mailbox); err != nil {
		return ReadManyResult{}, err
	}
	result := ReadManyResult{
		Kind:        "meshclaw_mail_read_many",
		Account:     publicAccount(account),
		Messages:    []Message{},
		GeneratedAt: time.Now().UTC(),
	}
	for _, id := range ids {
		message, readErr := client.fetchMessage(account.Mailbox, id, maxBody)
		if readErr != nil {
			result.Errors = append(result.Errors, MessageReadError{ID: id, Error: readErr.Error()})
			continue
		}
		result.Messages = append(result.Messages, message)
	}
	return result, nil
}

func DraftReply(accountID, messageID, intent string) (Draft, error) {
	message, err := Read(ReadOptions{Account: accountID, ID: messageID, MaxBody: 2000})
	if err != nil {
		return Draft{}, err
	}
	to := []string{}
	if message.Summary.From != "" {
		to = append(to, message.Summary.From)
	}
	subject := message.Summary.Subject
	if subject != "" && !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	body := draftReplyBody(message, intent)
	return SaveDraft(Draft{
		Account:      accountID,
		ThreadID:     message.Summary.ThreadID,
		To:           to,
		Subject:      subject,
		Body:         body,
		Status:       "draft",
		Policy:       "not sent; email_send requires approval and evidence",
		ApprovalHint: "Review and edit this draft, then create an approval record before any send adapter is allowed to transmit it.",
	})
}

func draftReplyBody(message Message, intent string) string {
	source := firstNonEmpty(message.Body, message.Summary.Snippet, message.Summary.Subject)
	plain := mailDraftPlainText(source, 1200)
	context := strings.ToLower(strings.Join([]string{message.Summary.Subject, plain, intent, message.Summary.From}, "\n"))
	signature := ""
	if strings.Contains(context, "홍길동") {
		signature = "\n홍길동 드림"
	}
	switch {
	case containsMailDraftAny(context, "m&a", "매물", "인수", "매각", "리스팅", "listing"):
		return strings.Join([]string{
			"안녕하세요.",
			"",
			"제안 주셔서 감사합니다. 보내주신 M&A 매물 자료는 확인했습니다.",
			"관심 키워드와 매물 조건을 검토해 보겠습니다.",
			"",
			"검토를 위해 각 매물의 상세 소개서, 최근 매출/영업이익 자료, 사용자·트래픽 지표, 매각 희망 조건을 함께 보내주시면 좋겠습니다.",
			"확인 후 관심 있는 항목이 있으면 다시 연락드리겠습니다.",
			"",
			"감사합니다." + signature,
		}, "\n")
	case containsMailDraftAny(context, "회의", "미팅", "meeting", "schedule", "일정"):
		return strings.Join([]string{
			"안녕하세요.",
			"",
			"메일 감사합니다. 제안해주신 일정과 내용을 확인했습니다.",
			"가능한 시간대를 확인한 뒤 회신드리겠습니다.",
			"",
			"회의 목적과 준비해야 할 자료가 있다면 함께 보내주세요.",
			"",
			"감사합니다." + signature,
		}, "\n")
	case containsMailDraftAny(context, "영수증", "인보이스", "invoice", "receipt", "결제"):
		return strings.Join([]string{
			"안녕하세요.",
			"",
			"보내주신 결제/영수증 관련 메일은 확인했습니다.",
			"내역을 검토한 뒤 필요한 조치가 있으면 회신드리겠습니다.",
			"",
			"감사합니다." + signature,
		}, "\n")
	default:
		if plain != "" {
			plain = trimText(plain, 220)
		}
		lines := []string{
			"안녕하세요.",
			"",
			"메일 감사합니다. 보내주신 내용은 확인했습니다.",
			"검토 후 필요한 사항이 있으면 회신드리겠습니다.",
		}
		if plain != "" {
			lines = append(lines, "", "확인한 내용: "+plain)
		}
		lines = append(lines, "", "감사합니다."+signature)
		return strings.Join(lines, "\n")
	}
}

func mailDraftPlainText(value string, max int) string {
	value = strings.TrimSpace(stripMultipartNoise(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	inTag := false
	for _, r := range value {
		switch r {
		case '<':
			inTag = true
			b.WriteRune(' ')
		case '>':
			inTag = false
			b.WriteRune(' ')
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	text := html.UnescapeString(b.String())
	text = strings.Join(strings.Fields(text), " ")
	return trimText(text, max)
}

func containsMailDraftAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func Compose(opts ComposeOptions) (Draft, error) {
	account, err := FindAccount(opts.Account)
	if err != nil {
		return Draft{}, err
	}
	to := cleanRecipients(opts.To)
	if len(to) == 0 {
		return Draft{}, errors.New("at least one recipient is required")
	}
	subject := strings.TrimSpace(opts.Subject)
	if subject == "" {
		return Draft{}, errors.New("subject is required")
	}
	body := strings.TrimSpace(opts.Body)
	if body == "" {
		return Draft{}, errors.New("body is required")
	}
	return SaveDraft(Draft{
		Account:      account.ID,
		To:           to,
		Subject:      subject,
		Body:         body,
		Status:       "draft",
		Policy:       "not sent; email_send requires approval and evidence",
		ApprovalHint: "Review this draft, then send only with explicit approval.",
	})
}

func Move(opts MutateOptions) (MutateResult, error) {
	return mutateMessages("move", opts)
}

func Delete(opts MutateOptions) (MutateResult, error) {
	return mutateMessages("delete", opts)
}

func mutateMessages(action string, opts MutateOptions) (MutateResult, error) {
	account, err := FindAccount(opts.Account)
	if err != nil {
		return MutateResult{}, err
	}
	if strings.TrimSpace(opts.Mailbox) != "" {
		account.Mailbox = strings.TrimSpace(opts.Mailbox)
	}
	ids := cleanMessageIDs(append(opts.IDs, opts.ID))
	result := MutateResult{
		Kind:             "meshclaw_mail_" + action,
		Account:          publicAccount(account),
		Action:           action,
		IDs:              ids,
		Target:           strings.TrimSpace(opts.Target),
		ApprovalRequired: true,
		Status:           "approval_required",
		GeneratedAt:      time.Now().UTC(),
	}
	if len(ids) == 0 {
		result.Error = "at least one message id is required"
		return result, errors.New(result.Error)
	}
	if action == "move" && result.Target == "" {
		result.Error = "target mailbox is required"
		return result, errors.New(result.Error)
	}
	if !opts.Approve {
		return result, nil
	}
	client, err := dialIMAP(account)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	defer client.close()
	if err := client.login(account); err != nil {
		result.Error = err.Error()
		return result, err
	}
	if err := client.selectMailbox(account.Mailbox); err != nil {
		result.Error = err.Error()
		return result, err
	}
	sequence := strings.Join(ids, ",")
	if action == "move" {
		err = client.moveUIDs(sequence, result.Target)
	} else {
		err = client.deleteUIDs(sequence)
	}
	if err != nil {
		result.Error = err.Error()
		result.Status = "failed"
		return result, err
	}
	result.Executed = true
	result.Status = "ok"
	return result, nil
}

func DownloadAttachments(opts AttachmentOptions) (AttachmentResult, error) {
	account, err := FindAccount(opts.Account)
	if err != nil {
		return AttachmentResult{}, err
	}
	if strings.TrimSpace(opts.Mailbox) != "" {
		account.Mailbox = strings.TrimSpace(opts.Mailbox)
	}
	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		dir = filepath.Join(DraftDir(), "..", "mail-attachments", sanitizeID(account.ID), sanitizeID(opts.ID))
	}
	result := AttachmentResult{
		Kind:             "meshclaw_mail_attachments",
		Account:          publicAccount(account),
		ID:               strings.TrimSpace(opts.ID),
		Dir:              dir,
		ApprovalRequired: true,
		GeneratedAt:      time.Now().UTC(),
	}
	if result.ID == "" {
		result.Error = "message id is required"
		return result, errors.New(result.Error)
	}
	if !opts.Approve {
		return result, nil
	}
	client, err := dialIMAP(account)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	defer client.close()
	if err := client.login(account); err != nil {
		result.Error = err.Error()
		return result, err
	}
	if err := client.selectMailbox(account.Mailbox); err != nil {
		result.Error = err.Error()
		return result, err
	}
	raw, err := client.fetchRawMessage(result.ID)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	files, err := saveAttachments(raw, dir)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	result.Files = files
	result.Executed = true
	return result, nil
}

func SendDraft(opts SendOptions) (SendResult, error) {
	draft, err := LoadDraft(opts.DraftID)
	if err != nil {
		return SendResult{Kind: "meshclaw_mail_send", DraftID: opts.DraftID, ApprovalRequired: true, Status: "failed", Error: err.Error(), GeneratedAt: time.Now().UTC()}, err
	}
	account, err := FindAccount(draft.Account)
	if err != nil {
		return SendResult{Kind: "meshclaw_mail_send", DraftID: draft.ID, ApprovalRequired: true, Status: "failed", Error: err.Error(), GeneratedAt: time.Now().UTC()}, err
	}
	result := SendResult{
		Kind:             "meshclaw_mail_send",
		DraftID:          draft.ID,
		Account:          publicAccount(account),
		To:               draft.To,
		Subject:          draft.Subject,
		ApprovalRequired: true,
		Status:           "approval_required",
		GeneratedAt:      time.Now().UTC(),
	}
	if !opts.Approve {
		return result, nil
	}
	password, err := accountPassword(account)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result, err
	}
	msg := buildSMTPMessage(account.Email, draft)
	err = sendSMTP(account, password, draft.To, msg)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result, err
	}
	result.Executed = true
	result.Status = "ok"
	return result, nil
}

func WatchOnce(opts WatchOptions) (WatchResult, error) {
	since := opts.Since
	if since <= 0 {
		since = 15 * time.Minute
	}
	search, err := Search(SearchOptions{Account: opts.Account, Since: time.Now().Add(-since), Limit: opts.Limit})
	if err != nil {
		return WatchResult{}, err
	}
	return WatchResult{
		Kind:        "meshclaw_mail_watch_once",
		Account:     search.Account,
		Since:       search.Since,
		Messages:    search.Messages,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

func cleanMessageIDs(ids []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range ids {
		for _, id := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '\t' || r == ' ' }) {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func cleanRecipients(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range values {
		for _, value := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '\t' }) {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func dialIMAP(account Account) (*imapClient, error) {
	address := net.JoinHostPort(account.Host, strconv.Itoa(account.Port))
	var conn net.Conn
	var err error
	if account.TLS {
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", address, &tls.Config{ServerName: account.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = net.DialTimeout("tcp", address, 15*time.Second)
	}
	if err != nil {
		return nil, err
	}
	client := &imapClient{conn: conn, reader: bufio.NewReader(conn)}
	if _, err := client.readLine(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

func (c *imapClient) login(account Account) error {
	password, err := accountPassword(account)
	if err != nil {
		return err
	}
	_, err = c.command("LOGIN %s %s", imapQuote(account.Username), imapQuote(password))
	return err
}

func (c *imapClient) selectMailbox(mailbox string) error {
	_, err := c.command("SELECT %s", imapQuote(mailbox))
	return err
}

func (c *imapClient) searchUIDs(opts SearchOptions) ([]string, error) {
	criteria := []string{"ALL"}
	if !opts.Since.IsZero() {
		criteria = append(criteria, "SINCE", opts.Since.Format("02-Jan-2006"))
	}
	if query := strings.TrimSpace(opts.Query); query != "" {
		criteria = append(criteria, "TEXT", imapQuote(query))
	}
	lines, err := c.command("UID SEARCH %s", strings.Join(criteria, " "))
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "*" && strings.EqualFold(fields[1], "SEARCH") {
			return fields[2:], nil
		}
	}
	return []string{}, nil
}

func (c *imapClient) fetchMessage(mailbox, uid string, maxBody int) (Message, error) {
	raw, err := c.fetchRawMessage(uid)
	if err != nil {
		return Message{}, err
	}
	if raw == "" {
		return Message{}, errors.New("message body was not returned")
	}
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return Message{}, err
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(msg.Body, int64(maxBody*2)))
	body := decodeBody(msg.Header.Get("Content-Transfer-Encoding"), string(bodyBytes))
	text, changed := summarizeBody(stripMultipartNoise(body), maxBody)
	headers := map[string]string{}
	for _, key := range []string{"From", "To", "Cc", "Subject", "Date", "Message-Id", "In-Reply-To", "References"} {
		if value := msg.Header.Get(key); value != "" {
			headers[strings.ToLower(key)] = decodeHeader(value)
		}
	}
	from := decodeHeader(msg.Header.Get("From"))
	to := parseAddressList(decodeHeader(msg.Header.Get("To")))
	subject := decodeHeader(msg.Header.Get("Subject"))
	date := messageDate(msg.Header.Get("Date"))
	return Message{
		Summary: MessageSummary{
			ID:        uid,
			ThreadID:  firstNonEmpty(headers["in-reply-to"], headers["message-id"], uid),
			Mailbox:   mailbox,
			From:      from,
			To:        to,
			Subject:   subject,
			Date:      date,
			Snippet:   trimText(text, 500),
			HasAttach: strings.Contains(strings.ToLower(raw), "content-disposition: attachment"),
			Redacted:  changed,
		},
		Body:     text,
		Headers:  headers,
		Redacted: changed,
	}, nil
}

func (c *imapClient) fetchRawMessage(uid string) (string, error) {
	lines, err := c.command("UID FETCH %s (BODY.PEEK[])", uid)
	if err != nil {
		return "", err
	}
	return extractIMAPLiteral(lines), nil
}

func (c *imapClient) moveUIDs(sequence, target string) error {
	if _, err := c.command("UID MOVE %s %s", sequence, imapQuote(target)); err == nil {
		return nil
	}
	_, err := c.command("UID COPY %s %s", sequence, imapQuote(target))
	if err != nil {
		return err
	}
	_, err = c.command("UID STORE %s +FLAGS.SILENT (\\Deleted)", sequence)
	if err != nil {
		return err
	}
	_, err = c.command("EXPUNGE")
	return err
}

func (c *imapClient) deleteUIDs(sequence string) error {
	if _, err := c.command("UID STORE %s +FLAGS.SILENT (\\Deleted)", sequence); err != nil {
		return err
	}
	_, err := c.command("EXPUNGE")
	return err
}

func (c *imapClient) command(format string, args ...interface{}) ([]string, error) {
	c.tag++
	tag := fmt.Sprintf("A%04d", c.tag)
	line := tag + " " + fmt.Sprintf(format, args...) + "\r\n"
	if _, err := c.conn.Write([]byte(line)); err != nil {
		return nil, err
	}
	lines := []string{}
	for {
		read, err := c.readLine()
		if err != nil {
			return lines, err
		}
		lines = append(lines, read)
		if strings.HasPrefix(read, tag+" ") {
			upper := strings.ToUpper(read)
			if strings.Contains(upper, " OK") || strings.HasPrefix(upper, tag+" OK") {
				return lines, nil
			}
			return lines, fmt.Errorf("imap command failed: %s", read)
		}
	}
}

func (c *imapClient) readLine() (string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if open := strings.LastIndex(line, "{"); open >= 0 && strings.HasSuffix(line, "}") {
		sizeText := strings.TrimSuffix(line[open+1:], "}")
		if size, err := strconv.Atoi(sizeText); err == nil && size > 0 {
			buf := make([]byte, size)
			if _, err := io.ReadFull(c.reader, buf); err != nil {
				return line, err
			}
			line += "\n" + string(buf)
			rest, _ := c.reader.ReadString('\n')
			line += strings.TrimRight(rest, "\r\n")
		}
	}
	return line, nil
}

func (c *imapClient) close() {
	_, _ = c.command("LOGOUT")
	_ = c.conn.Close()
}

func imapQuote(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", " ")
	return `"` + value + `"`
}

func extractIMAPLiteral(lines []string) string {
	for _, line := range lines {
		if idx := strings.Index(line, "\n"); idx >= 0 {
			body := line[idx+1:]
			if end := strings.LastIndex(body, "\n)"); end >= 0 {
				body = body[:end]
			}
			return strings.TrimSpace(body)
		}
	}
	return ""
}

func decodeHeader(value string) string {
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(value)
	if err == nil {
		return strings.TrimSpace(decoded)
	}
	return strings.TrimSpace(value)
}

func decodeBody(encoding, body string) string {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(body), ""))
		if err == nil {
			return string(decoded)
		}
	case "quoted-printable":
		reader := quotedPrintableReader(body)
		decoded, err := io.ReadAll(reader)
		if err == nil {
			return string(decoded)
		}
	}
	return body
}

func saveAttachments(raw, dir string) ([]AttachmentFile, error) {
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return nil, err
	}
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		return []AttachmentFile{}, nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	reader := multipart.NewReader(msg.Body, params["boundary"])
	files := []AttachmentFile{}
	if err := walkMultipart(reader, dir, &files); err != nil {
		return files, err
	}
	return files, nil
}

func walkMultipart(reader *multipart.Reader, dir string, files *[]AttachmentFile) error {
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		contentType := part.Header.Get("Content-Type")
		mediaType, params, _ := mime.ParseMediaType(contentType)
		if strings.HasPrefix(strings.ToLower(mediaType), "multipart/") && params["boundary"] != "" {
			if err := walkMultipart(multipart.NewReader(part, params["boundary"]), dir, files); err != nil {
				return err
			}
			continue
		}
		filename := part.FileName()
		if filename == "" {
			continue
		}
		filename = safeFilename(filename)
		path := filepath.Join(dir, filename)
		path = uniquePath(path)
		out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		written, copyErr := io.Copy(out, part)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		*files = append(*files, AttachmentFile{Filename: filepath.Base(path), ContentType: contentType, Size: written, Path: path})
	}
}

func safeFilename(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	if value == "." || value == "/" || value == "" {
		return "attachment"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "attachment"
	}
	return out
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func buildSMTPMessage(from string, draft Draft) []byte {
	headers := []string{
		"From: " + from,
		"To: " + strings.Join(draft.To, ", "),
		"Subject: " + mime.QEncoding.Encode("utf-8", draft.Subject),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + draft.Body + "\r\n")
}

func sendSMTP(account Account, password string, to []string, msg []byte) error {
	host := firstNonEmpty(account.SMTPHost, account.Host)
	port := account.SMTPPort
	if port == 0 {
		port = 465
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	auth := smtp.PlainAuth("", account.Username, password, host)
	if account.SMTPTLS || port == 465 {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 20 * time.Second}, "tcp", address, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer client.Close()
		if err := client.Auth(auth); err != nil {
			return err
		}
		if err := client.Mail(account.Email); err != nil {
			return err
		}
		for _, rcpt := range to {
			if err := client.Rcpt(rcpt); err != nil {
				return err
			}
		}
		writer, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := writer.Write(msg); err != nil {
			_ = writer.Close()
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}
		return client.Quit()
	}
	return smtp.SendMail(address, auth, account.Email, to, msg)
}

func quotedPrintableReader(body string) io.Reader {
	return quotedprintable.NewReader(strings.NewReader(body))
}

func stripMultipartNoise(body string) string {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lower, "--") || strings.HasPrefix(lower, "content-type:") || strings.HasPrefix(lower, "content-transfer-encoding:") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func parseAddressList(value string) []string {
	list, err := mail.ParseAddressList(value)
	if err != nil {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{strings.TrimSpace(value)}
	}
	out := make([]string, 0, len(list))
	for _, address := range list {
		out = append(out, address.String())
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
