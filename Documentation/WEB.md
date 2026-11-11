# OfficeLite on the web

WordLite and ExcelLite also run in the browser: the desktop app built for
WebAssembly, served by OfficeLite's own web server (`cmd/officeweb`). One
site (for example `https://office.joshco.com`) serves everything;
`word.` and `excel.` hostnames can redirect to its `/word` and `/excel`
pages.

- [Accounts](#accounts)
- [Documents on the server](#documents-on-the-server)
- [File name rules](#file-name-rules)
- [Size limits](#size-limits)
- [Logging out](#logging-out)
- [Running the server](#running-the-server)
- [Limitations](#limitations)

## Accounts

Only administrators add users (Users page, or `officeweb -create-admin` on
the server); there is no sign-up.

Usernames:

- 1 to 64 characters from `a`–`z`, `A`–`Z`, `0`–`9`, `.`, `_` and `-`,
  starting with a letter or digit.
- Case is kept but ignored: `Josh` is shown as typed, and logs in as
  `josh` or `JOSH`; no second account can be `josh`.

Passwords have at least 12 characters and at most 72 bytes.

Deleting a user (after a Yes/No question) deletes all of their documents
too. If their folder can't be removed, the page says so, and no new account
of that name can be made until an administrator removes the folder by hand,
so nobody inherits someone else's files.

## Documents on the server

Each user has their own documents on the server, 1 GiB by default. In the
apps they look like a folder named `/server`:

- **File → Open → Browse** shows your server files (the ones the app can
  open). **From this computer…** opens the browser's file picker instead.
- **File → Save As** asks for a name on the server; a name typed without
  an extension gets the format's. Saving over an existing file asks first.
  **Download to this computer** saves by downloading instead.
- **Save** (Ctrl+S) writes a server document back in place.
- **AutoSave is on by default** in the browser, so server documents save
  themselves a few seconds after each change. (On the desktop it stays off
  until you turn it on.)

The **Files** page lists your documents with their sizes, and lets you
upload (several at once; tick *Replace* to overwrite files of the same
name), download, rename and delete them (delete asks Yes/No). Clicking a
document's name opens it in WordLite or ExcelLite.

Documents from your computer still work as before: open them from your
disk and save by downloading.

## File name rules

Every document stored on the server follows these rules. The Files page,
the Save As dialog and the server all check them.

| Rule | Detail |
|------|--------|
| Length | 1 to **64 characters**, extension included. |
| Spaces | Leading and trailing spaces are **trimmed** from what you type; spaces inside a name are fine (`Budget 2026.xlsx`). |
| Characters | **Printable ASCII only**: codes 040–0176 octal (letters, digits, space and punctuation). |
| No control characters | Nothing below 040 octal (tab, newline, NUL, …) and no DEL (0177). |
| No invisible or non-ASCII characters | Nothing above 0177 octal: no accented letters, no zero-width or other invisible characters, no look-alikes. |
| Forbidden characters | `/`, `\` and `*`. |
| Forbidden starts | A name may not start with `.` (hidden and temporary files) or `~$` (Office lock files). |
| Extension | Must end in one of `.docx .docm .dotx .dotm .doc .dot .pages .xlsx .xlsm .xls .csv .tsv .numbers .txt .pdf` (any case). |
| Case | Ignored when comparing: `Budget.xlsx` and `budget.xlsx` are the same file, which keeps the case it was first saved with. |

## Size limits

| Limit | Value |
|-------|-------|
| Storage per user | 1 GiB (all of the user's documents together) |
| One document | 256 MiB |
| Time to upload one document | 10 minutes |

A save or upload over a limit is refused whole; the document already on
the server is left as it was. The apps show the server's reason.

## Logging out

After **1 hour without a key press, click or touch** in any of your tabs,
the site warns you for 2 minutes and then logs you out, coming back to the
same page when you log in again. While you are working (even only typing in
a document), the page keeps your session alive. Unsaved changes to
documents from your computer are lost at an idle logout; server documents
with AutoSave on are already saved.

## Running the server

```sh
./web.sh run           # build and serve on http://localhost:7100 (data in bin/webdata/)
./web.sh admin NAME    # add an administrator
./web.sh package       # build bin/officeweb-<version>.tar.gz for a Linux server
```

The data directory (`-data`) holds the database `officeweb.db` and the
documents, one folder per user, named after the username in lowercase:

```
data/
  officeweb.db        users, sessions, login attempts
  users/
    josh/
      Budget.xlsx
      Memo.docx
```

The documents are ordinary files under their own names: back up the whole
data directory (the database and `users/`) and restore it the same way. A
save writes a hidden temporary file (`.upload-…`) in the user's folder and
renames it over the document, so a document is never left half written.

`web/deploy/` has a sample Apache configuration and systemd unit. All the
server's limits are named constants in `web/limits.go`; the username and
file-name rules are in `web/names`.

## Limitations

- Two tabs (or two people with the same account) editing the same server
  document: the last save wins. There is no lock across tabs.
- Server storage is a single folder per user: no subfolders.
- Only documents are stored (the extensions above), not pictures or other
  files. Inserting a picture still uses the browser's file picker.
