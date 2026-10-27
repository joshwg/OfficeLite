# Fonts

OfficeLite loads every `.ttf` / `.otf` / `.ttc` in this folder at runtime, in
addition to the operating system's fonts. It looks for a `Fonts` folder next
to the executable and in the current directory, plus any folders listed in
`OFFICELITE_FONTS`.

The font files here are **not committed** (see `.gitignore`). They were
copied locally from an old Windows font folder (`D:\archive\ackee\D\Archive\Fonts`,
TrueType only; bitmap `.fon` files can't be rendered). They are Microsoft and
other vendors' fonts: fine to use on this machine, but they may not be
committed or shipped with OfficeLite builds.

Only the families a document uses are loaded; the folder is indexed by reading
each file's name table, and the index is cached in the user cache directory
(`officelite/fontindex.json`).

Free, metric-compatible substitutes that *can* be shipped (SIL OFL /
Apache): Liberation Sans/Serif/Mono (Arial, Times New Roman, Courier New),
Carlito (Calibri), Caladea (Cambria). OfficeLite falls back to them
automatically when the Microsoft font is missing.
