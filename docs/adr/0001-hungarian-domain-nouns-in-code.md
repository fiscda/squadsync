# Hungarian domain nouns in code identifiers

The app is built for Hungarian friend groups, and its UI is Hungarian only. We decided that code uses the Hungarian domain nouns from `GLOSSARY.md` without accents (`Csoport`, `Esemeny`, `Felhasznalo`, `Idopont`, `Visszajelzes`), and that everything else in code stays English. Code and conversation then share one vocabulary, and nobody has to translate between "Esemény" in a discussion and `Event` in the code.

## Considered Options

- **English identifiers with a glossary mapping**: rejected, because every conversation about the domain would need a translation step, and the English words (event, session, slot) are ambiguous in ways the Hungarian ones aren't here.
- **Fully Hungarian code**: rejected, because mixing Hungarian verbs and helpers with English libraries reads worse than keeping domain nouns as the only Hungarian words.

## Consequences

Identifiers drop the accents (`Esemény` → `Esemeny`, `Időpont` → `Idopont`), so the glossary's accented form and the code's form differ only in the accents.
