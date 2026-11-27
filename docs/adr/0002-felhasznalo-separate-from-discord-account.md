# Felhasználó is separate from the Discord account, and Go owns the linking

Some people use separate Discord accounts on PC and on phone. We decided that a Felhasználó is our own identity, with one **main** Discord account and any number of **linked** ones, rather than treating each Discord account as a user. A Visszajelzés from any of their accounts counts as that Felhasználó's single answer.

Better Auth's support for linking two accounts from the same provider (`linkSocial`) isn't documented. Rather than depend on it, **Better Auth handles only web login with the main account**, and a **Go-owned table** maps every Discord account, the main one included, to its Felhasználó.

## Considered Options

- **Better Auth `linkSocial` for the second account**: rejected. It's undocumented for two accounts from one provider and would need a spike to trust.
- **Let any linked account log in to the web**, mapping several Better Auth users to one Felhasználó: rejected. It would split "Better Auth user" and "Felhasználó" into two things on every request. The real pain was Discord on two devices, and any browser can log in with the main account.

## Consequences

- **First login.** Right after a Felhasználó's first web login, Next.js calls Go. Go creates the Felhasználó with the same id as Better Auth's `user.id` and records the main Discord id.
- **Linking.** Linking runs a Discord OAuth flow for the second account. The callback goes through a Next.js route to Go, which records the link.
- **No merging.** Linking is refused ("Ez a fiók már használatban van.") if that Discord account already belongs to a Felhasználó with a Tag or any data.
- **Logging in with a linked account is refused.** A Better Auth hook asks Go before creating a user, and refuses if the Discord id is already linked: "Ez a fiók X-hez van kapcsolva, lépj be azzal."
- **DMs go to every account, on purpose.** Someone with two accounts gets each DM twice. That way a message always reaches whichever device they are using. It isn't a bug.
- **Every bot interaction resolves the Discord account first,** through Go's table, before doing anything. An unknown Discord account gets a login link, and nothing is recorded.
