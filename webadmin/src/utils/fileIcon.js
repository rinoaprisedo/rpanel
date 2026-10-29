import {
  Folder, FolderGit2, FolderLock, FolderCog, FolderArchive, FolderSymlink,
  File, FileText, FileCode, FileJson, FileImage, FileArchive, FileAudio, FileVideo, FileSpreadsheet,
  FileKey, FileLock, FileTerminal, FileCog, FileSymlink, FileType, Database, ScrollText,
} from 'lucide-react'

// Icon + color + label per kind of entry, so the file list can be scanned by eye.
// Class names are written out in full so Tailwind keeps them.
const kind = (Icon, color, label) => ({ Icon, color, label })

const FOLDER = kind(Folder, 'text-amber-500', 'Folder')
const HIDDEN_FOLDER = kind(Folder, 'text-amber-500/50', 'Hidden folder')
const SPECIAL_FOLDERS = {
  '.git': kind(FolderGit2, 'text-orange-500', 'Git repository data'),
  node_modules: kind(FolderArchive, 'text-lime-500', 'Dependencies'),
  vendor: kind(FolderArchive, 'text-lime-500', 'Dependencies'),
  '.ssh': kind(FolderLock, 'text-red-500', 'SSH keys'),
  ssl: kind(FolderLock, 'text-red-500', 'Certificates and keys'),
  private: kind(FolderLock, 'text-red-500', 'Private files'),
  certs: kind(FolderLock, 'text-red-500', 'Certificates and keys'),
  etc: kind(FolderCog, 'text-sky-500', 'Configuration'),
  config: kind(FolderCog, 'text-sky-500', 'Configuration'),
  conf: kind(FolderCog, 'text-sky-500', 'Configuration'),
  'conf.d': kind(FolderCog, 'text-sky-500', 'Configuration'),
  log: kind(FolderArchive, 'text-zinc-400', 'Logs'),
  logs: kind(FolderArchive, 'text-zinc-400', 'Logs'),
}

const SECRET = kind(FileKey, 'text-red-500', 'Secret / credentials')
const CODE = (color, label) => kind(FileCode, color, label)
const BY_EXT = {
  // images, media
  png: kind(FileImage, 'text-violet-500', 'Image'), jpg: kind(FileImage, 'text-violet-500', 'Image'), jpeg: kind(FileImage, 'text-violet-500', 'Image'),
  gif: kind(FileImage, 'text-violet-500', 'Image'), webp: kind(FileImage, 'text-violet-500', 'Image'), svg: kind(FileImage, 'text-violet-500', 'Image'),
  ico: kind(FileImage, 'text-violet-500', 'Image'), bmp: kind(FileImage, 'text-violet-500', 'Image'), avif: kind(FileImage, 'text-violet-500', 'Image'),
  mp4: kind(FileVideo, 'text-pink-500', 'Video'), mkv: kind(FileVideo, 'text-pink-500', 'Video'), mov: kind(FileVideo, 'text-pink-500', 'Video'), webm: kind(FileVideo, 'text-pink-500', 'Video'),
  mp3: kind(FileAudio, 'text-fuchsia-500', 'Audio'), wav: kind(FileAudio, 'text-fuchsia-500', 'Audio'), ogg: kind(FileAudio, 'text-fuchsia-500', 'Audio'), flac: kind(FileAudio, 'text-fuchsia-500', 'Audio'),
  // archives
  zip: kind(FileArchive, 'text-orange-500', 'Archive'), tar: kind(FileArchive, 'text-orange-500', 'Archive'), gz: kind(FileArchive, 'text-orange-500', 'Archive'),
  tgz: kind(FileArchive, 'text-orange-500', 'Archive'), bz2: kind(FileArchive, 'text-orange-500', 'Archive'), xz: kind(FileArchive, 'text-orange-500', 'Archive'),
  '7z': kind(FileArchive, 'text-orange-500', 'Archive'), rar: kind(FileArchive, 'text-orange-500', 'Archive'),
  // code
  html: CODE('text-orange-600', 'HTML'), htm: CODE('text-orange-600', 'HTML'), vue: CODE('text-emerald-500', 'Vue'),
  css: CODE('text-sky-500', 'CSS'), scss: CODE('text-pink-400', 'Sass'), less: CODE('text-sky-400', 'Less'),
  js: CODE('text-yellow-500', 'JavaScript'), mjs: CODE('text-yellow-500', 'JavaScript'), cjs: CODE('text-yellow-500', 'JavaScript'), jsx: CODE('text-cyan-400', 'React'),
  ts: CODE('text-blue-500', 'TypeScript'), tsx: CODE('text-cyan-400', 'React'),
  php: CODE('text-indigo-400', 'PHP'), py: CODE('text-blue-400', 'Python'), go: CODE('text-cyan-500', 'Go'), rb: CODE('text-red-400', 'Ruby'),
  java: CODE('text-red-500', 'Java'), c: CODE('text-blue-500', 'C'), cpp: CODE('text-blue-500', 'C++'), rs: CODE('text-orange-500', 'Rust'),
  xml: CODE('text-orange-400', 'XML'),
  sh: kind(FileTerminal, 'text-green-500', 'Shell script'), bash: kind(FileTerminal, 'text-green-500', 'Shell script'), zsh: kind(FileTerminal, 'text-green-500', 'Shell script'),
  // data, config
  json: kind(FileJson, 'text-lime-500', 'JSON'),
  yml: kind(FileCog, 'text-teal-500', 'YAML'), yaml: kind(FileCog, 'text-teal-500', 'YAML'), toml: kind(FileCog, 'text-teal-500', 'TOML'),
  ini: kind(FileCog, 'text-sky-500', 'Config'), conf: kind(FileCog, 'text-sky-500', 'Config'), cfg: kind(FileCog, 'text-sky-500', 'Config'),
  service: kind(FileCog, 'text-sky-500', 'systemd unit'), timer: kind(FileCog, 'text-sky-500', 'systemd timer'), htaccess: kind(FileCog, 'text-sky-500', 'Apache config'),
  env: SECRET, pem: SECRET, key: SECRET, crt: kind(FileKey, 'text-amber-500', 'Certificate'), csr: kind(FileKey, 'text-amber-500', 'Certificate request'),
  sql: kind(Database, 'text-teal-500', 'SQL'), sqlite: kind(Database, 'text-teal-500', 'Database'), db: kind(Database, 'text-teal-500', 'Database'),
  csv: kind(FileSpreadsheet, 'text-emerald-500', 'Spreadsheet'), xls: kind(FileSpreadsheet, 'text-emerald-500', 'Spreadsheet'), xlsx: kind(FileSpreadsheet, 'text-emerald-500', 'Spreadsheet'),
  pdf: kind(FileType, 'text-red-400', 'PDF'), doc: kind(FileText, 'text-blue-500', 'Document'), docx: kind(FileText, 'text-blue-500', 'Document'),
  md: kind(FileText, 'text-slate-400', 'Markdown'), txt: kind(FileText, 'text-slate-400', 'Text'),
  log: kind(ScrollText, 'text-zinc-400', 'Log'), lock: kind(FileLock, 'text-zinc-400', 'Lock file'),
}
const BY_NAME = {
  dockerfile: kind(FileCog, 'text-sky-500', 'Dockerfile'), makefile: kind(FileTerminal, 'text-green-500', 'Makefile'), crontab: kind(FileTerminal, 'text-green-500', 'Cron table'),
  '.htaccess': BY_EXT.htaccess, '.gitignore': kind(FileText, 'text-orange-500/70', 'Git ignore list'), 'id_rsa': SECRET, 'id_ed25519': SECRET, authorized_keys: SECRET,
}
const FILE = kind(File, 'text-text-secondary', 'File')

// e: an entry from the file manager API ({ name, is_dir, is_link, ... }).
export function iconFor(e) {
  const name = (e.name || '').toLowerCase()
  if (e.is_dir) {
    if (e.is_link) return kind(FolderSymlink, 'text-cyan-500', 'Folder link')
    return SPECIAL_FOLDERS[name] || (name.startsWith('.') ? HIDDEN_FOLDER : FOLDER)
  }
  if (e.is_link) return kind(FileSymlink, 'text-cyan-500', 'Link')
  if (BY_NAME[name]) return BY_NAME[name]
  if (name === '.env' || name.startsWith('.env.')) return SECRET
  const ext = name.includes('.') ? name.split('.').pop() : ''
  return BY_EXT[ext] || FILE
}
