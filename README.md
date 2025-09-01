# maildir2pdf

A Go command-line tool that scans Maildir directories for emails with PDF attachments and extracts them to the current working directory.

## Features

- **Complete Maildir scanning**: Scans all mailboxes (INBOX, Sent, Drafts, Trash, custom folders) 
- **PDF extraction**: Finds and extracts PDF attachments from emails
- **Proper decoding**: Handles base64 and other transfer encodings
- **Timestamp preservation**: Sets extracted PDF timestamps to match email dates
- **Filename handling**: Sanitizes filenames and avoids collisions with numeric suffixes
- **Symlink safety**: Does not follow symbolic links during scanning
- **Mailbox context**: Shows which mailbox contained each PDF in output

## Installation

```bash
go build
```

## Usage

```bash
./maildir2pdf -maildir /path/to/maildir
```

### Example

```bash
./maildir2pdf -maildir ~/Maildir
```

Output:
```
Saved PDF: /current/dir/document.pdf (from ~/Maildir/.Sent/cur/1234567890.email in mailbox Sent)
Saved PDF: /current/dir/report.pdf (from ~/Maildir/cur/1234567891.email in mailbox INBOX)
```

## How it Works

1. **Mailbox Discovery**: Recursively finds all valid mailbox directories containing `cur`, `new`, or `tmp` subdirectories
2. **Email Processing**: Parses each email file using Go's `net/mail` package
3. **Attachment Detection**: Identifies PDF attachments by Content-Type `application/pdf`
4. **Content Decoding**: Properly decodes base64 and other transfer encodings
5. **File Creation**: Saves PDFs to current directory with original filenames
6. **Timestamp Setting**: Sets file modification time to email date

## Maildir Structure Support

The tool supports standard Maildir structure:
```
Maildir/
├── cur/           # Current messages (INBOX)
├── new/           # New messages (INBOX)  
├── tmp/           # Temporary files (INBOX)
├── .Sent/         # Sent mailbox
│   ├── cur/
│   ├── new/
│   └── tmp/
├── .Drafts/       # Drafts mailbox
│   ├── cur/
│   ├── new/
│   └── tmp/
└── .Trash/        # Trash mailbox
    ├── cur/
    ├── new/
    └── tmp/
```

## Security Features

- **No symlink following**: Prevents directory traversal attacks
- **Filename sanitization**: Removes dangerous characters from filenames
- **Safe file writing**: Uses secure file creation methods

## Error Handling

- Gracefully handles malformed emails
- Continues processing if individual emails fail
- Logs warnings for non-critical errors
- Reports specific error messages for debugging

## Requirements

- Go 1.21 or later
- Valid Maildir structure
- Read permissions on maildir files

## License

This tool is provided under the GNU Affero General Public License, version 3
