package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	var maildirPath string
	flag.StringVar(&maildirPath, "maildir", "", "Path to the maildir to scan")
	flag.Parse()

	if maildirPath == "" {
		log.Fatal("Please specify a maildir path using -maildir flag")
	}

	if err := scanMaildir(maildirPath); err != nil {
		log.Fatal("Error scanning maildir:", err)
	}
}

func scanMaildir(maildirPath string) error {
	mailboxes, err := discoverMailboxes(maildirPath)
	if err != nil {
		return fmt.Errorf("error discovering mailboxes: %v", err)
	}
	
	for _, mailbox := range mailboxes {
		if err := scanSingleMailbox(mailbox.Path, mailbox.Name); err != nil {
			log.Printf("Error scanning mailbox %s: %v", mailbox.Name, err)
		}
	}
	
	return nil
}

type Mailbox struct {
	Name string
	Path string
}

func discoverMailboxes(maildirPath string) ([]Mailbox, error) {
	var mailboxes []Mailbox
	
	// Add the main inbox
	if isValidMailbox(maildirPath) {
		mailboxes = append(mailboxes, Mailbox{Name: "INBOX", Path: maildirPath})
	}
	
	// Discover all subdirectories that are valid mailboxes
	err := filepath.Walk(maildirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		
		if !info.IsDir() || path == maildirPath {
			return nil
		}
		
		if isValidMailbox(path) {
			relPath, err := filepath.Rel(maildirPath, path)
			if err != nil {
				return err
			}
			
			// Clean up mailbox name (remove leading dots, replace path separators)
			name := strings.ReplaceAll(relPath, string(filepath.Separator), "/")
			if strings.HasPrefix(name, ".") {
				name = name[1:] // Remove leading dot
			}
			
			mailboxes = append(mailboxes, Mailbox{Name: name, Path: path})
		}
		
		return nil
	})
	
	return mailboxes, err
}

func isValidMailbox(path string) bool {
	subdirs := []string{"cur", "new", "tmp"}
	
	for _, subdir := range subdirs {
		dirPath := filepath.Join(path, subdir)
		if _, err := os.Stat(dirPath); err == nil {
			return true
		}
	}
	
	return false
}

func scanSingleMailbox(mailboxPath, mailboxName string) error {
	subdirs := []string{"cur", "new", "tmp"}
	
	for _, subdir := range subdirs {
		dirPath := filepath.Join(mailboxPath, subdir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			continue
		}
		
		err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			
			// Skip symlinks
			if info.Mode()&os.ModeSymlink != 0 {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			
			if !info.IsDir() {
				return processEmailFile(path, mailboxName)
			}
			return nil
		})
		
		if err != nil {
			return fmt.Errorf("error walking directory %s: %v", dirPath, err)
		}
	}
	
	return nil
}

func processEmailFile(filePath, mailboxName string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening file %s: %v", filePath, err)
	}
	defer file.Close()

	msg, err := mail.ReadMessage(file)
	if err != nil {
		return fmt.Errorf("error parsing email %s: %v", filePath, err)
	}

	return extractPDFAttachments(msg, filePath, mailboxName)
}

func extractPDFAttachments(msg *mail.Message, emailPath, mailboxName string) error {
	// Parse email date
	var emailTime time.Time
	if dateStr := msg.Header.Get("Date"); dateStr != "" {
		if parsedTime, err := mail.ParseDate(dateStr); err == nil {
			emailTime = parsedTime
		}
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		return nil
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return nil
		}

		reader := multipart.NewReader(msg.Body, boundary)
		
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("error reading multipart: %v", err)
			}

			if err := processPart(part, emailPath, mailboxName, emailTime); err != nil {
				log.Printf("Error processing part: %v", err)
			}
			part.Close()
		}
	} else if mediaType == "application/pdf" {
		encoding := msg.Header.Get("Content-Transfer-Encoding")
		return savePDFAttachmentWithEncoding(msg.Body, "attachment.pdf", emailPath, mailboxName, encoding, emailTime)
	}

	return nil
}

func processPart(part *multipart.Part, emailPath, mailboxName string, emailTime time.Time) error {
	contentType := part.Header.Get("Content-Type")
	contentDisposition := part.Header.Get("Content-Disposition")
	
	if strings.Contains(contentType, "application/pdf") {
		filename := extractFilename(contentDisposition, part.Header.Get("Content-Type"))
		if filename == "" {
			filename = "attachment.pdf"
		}
		
		encoding := part.Header.Get("Content-Transfer-Encoding")
		return savePDFAttachmentWithEncoding(part, filename, emailPath, mailboxName, encoding, emailTime)
	}
	
	if strings.HasPrefix(contentType, "multipart/") {
		mediaType, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return err
		}
		
		if strings.HasPrefix(mediaType, "multipart/") {
			boundary := params["boundary"]
			if boundary != "" {
				reader := multipart.NewReader(part, boundary)
				for {
					subPart, err := reader.NextPart()
					if err == io.EOF {
						break
					}
					if err != nil {
						return err
					}
					
					processPart(subPart, emailPath, mailboxName, emailTime)
					subPart.Close()
				}
			}
		}
	}
	
	return nil
}

func extractFilename(contentDisposition, contentType string) string {
	if contentDisposition != "" {
		_, params, err := mime.ParseMediaType(contentDisposition)
		if err == nil {
			if filename := params["filename"]; filename != "" {
				return filename
			}
		}
	}
	
	if contentType != "" {
		_, params, err := mime.ParseMediaType(contentType)
		if err == nil {
			if filename := params["name"]; filename != "" {
				return filename
			}
		}
	}
	
	return ""
}

func savePDFAttachmentWithEncoding(reader io.Reader, filename, emailPath, mailboxName, encoding string, emailTime time.Time) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("error reading attachment data: %v", err)
	}

	var decodedData []byte
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		// Clean up base64 data by removing whitespace/newlines
		cleanData := strings.ReplaceAll(string(data), "\n", "")
		cleanData = strings.ReplaceAll(cleanData, "\r", "")
		cleanData = strings.ReplaceAll(cleanData, " ", "")
		
		decodedData, err = base64.StdEncoding.DecodeString(cleanData)
		if err != nil {
			return fmt.Errorf("error decoding base64 data: %v", err)
		}
	case "quoted-printable":
		// Handle quoted-printable encoding if needed
		decodedData = data
	default:
		// No encoding or binary
		decodedData = data
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current directory: %v", err)
	}

	filename = sanitizeFilename(filename)
	outputPath := filepath.Join(cwd, filename)
	
	counter := 1
	for {
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			break
		}
		
		ext := filepath.Ext(filename)
		name := strings.TrimSuffix(filename, ext)
		outputPath = filepath.Join(cwd, fmt.Sprintf("%s_%d%s", name, counter, ext))
		counter++
	}

	err = os.WriteFile(outputPath, decodedData, 0644)
	if err != nil {
		return fmt.Errorf("error writing PDF file %s: %v", outputPath, err)
	}

	// Set file timestamp to email date if available
	if !emailTime.IsZero() {
		err = os.Chtimes(outputPath, emailTime, emailTime)
		if err != nil {
			log.Printf("Warning: could not set timestamp for %s: %v", outputPath, err)
		}
	}

	fmt.Printf("Saved PDF: %s (from %s in mailbox %s)\n", outputPath, emailPath, mailboxName)
	return nil
}

func sanitizeFilename(filename string) string {
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")
	filename = strings.ReplaceAll(filename, ":", "_")
	filename = strings.ReplaceAll(filename, "*", "_")
	filename = strings.ReplaceAll(filename, "?", "_")
	filename = strings.ReplaceAll(filename, "\"", "_")
	filename = strings.ReplaceAll(filename, "<", "_")
	filename = strings.ReplaceAll(filename, ">", "_")
	filename = strings.ReplaceAll(filename, "|", "_")
	
	if filename == "" {
		filename = "attachment.pdf"
	}
	
	return filename
}