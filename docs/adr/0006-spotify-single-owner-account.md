# Every Spotify playlist is owned by one person's account

Spotify's development mode allows only 5 authorized users per app and requires the owner to have Premium. Since 2025-05-15, Extended Quota is open only to organizations. So friends can't each log in with Spotify. Instead, **one owner account (Premium) authorizes the app once**. Every Esemény's Lejátszási lista is created on that account, and songs that Résztvevők submit (as Spotify track links, from Discord or the web) are added with the owner's token.

## Considered Options

- **Each Résztvevő authorizes Spotify**: impossible beyond 5 people, and Extended Quota isn't available to individuals.
- **Store a link to a hand-made collaborative playlist**: kept as the fallback if the owner model ever breaks. It has no API dependency, but loses "add songs from Discord".

## Consequences

- The owner's Spotify account fills up with the app's playlists. If that account loses Premium or revokes access, the feature stops.
- Use the post-Feb-2026 endpoints: `POST /me/playlists` and `POST /playlists/{id}/items`. The `/tracks` and `/users/{id}/playlists` variants were removed.
