# Squad Sync

Helps a small, recurring group of friends find a time to meet, play or travel together, commit to it, and organise what the gathering needs.

Terms are Hungarian; definitions are English. In code, the same nouns are used without accents (`Csoport`, `Esemeny`, `Felhasznalo`).

## Language

### People and groups

**Felhasználó**:
A single human using the app, identified by one or more linked Discord accounts. Whichever account they act from, it is still one Felhasználó.
_Avoid_: User account, profile, person

**Csoport**:
A persistent group of Felhasználók with stable membership who meet repeatedly. Each Csoport has its own name, which is what people usually see.
_Avoid_: Squad, Csapat, group, team, crew

**Tag**:
One Felhasználó's membership in one Csoport. A Felhasználó in three Csoports has three Tags.
_Avoid_: Member (when it means the person rather than the membership)

**Admin**:
A role held by a Tag that lets them manage the Csoport itself, such as its name, its Tags and its linked Discord channel. A Csoport can have several, and being Admin gives no authority over any Esemény.
_Avoid_: Owner, moderator, Gazda, Vezető

**Vendég**:
Someone who attends one specific Esemény without being a Tag of its Csoport. A Vendég is either a Felhasználó who joined through an Esemény-meghívó, or just a name added by a Tag.
_Avoid_: Guest, +1, outsider

**Csoport-meghívó**:
A reusable link that makes whoever opens it and logs in a Tag of one Csoport. It never expires by default.
_Avoid_: Meghívó (alone), invite, join link

**Csatorna**:
The one Discord channel an Admin has linked to a Csoport, where the bot posts the Csoport's public moments. A Csoport has at most one; without one, everything goes by DM.
_Avoid_: Channel, Szoba (that word belongs to Szállás)

**Esemény-meghívó**:
A reusable link that makes whoever opens it and logs in a Vendég of one Esemény. It stops working once the Esemény is Lezajlott or Elmaradt.
_Avoid_: Meghívó (alone), Vendéglink, guest link

### Scheduling

**Esemény**:
A single gathering of one Csoport. It belongs to exactly one Csoport, and Vendégs can attend as well as Tags.
_Avoid_: Session, event, meetup, hangout

**Szervező**:
The one Felhasználó responsible for an Esemény. It starts as whoever proposed the Esemény and can be handed to someone else. Only the Szervező confirms, reopens or cancels the Esemény.
_Avoid_: Organizer, proposer, host, owner

**Egyeztetés** / **Véglegesítve** / **Elmaradt** / **Lezajlott**:
The states of an Esemény. Egyeztetés means Időpontok are being answered, Véglegesítve means one Időpont is confirmed, Elmaradt means it was cancelled, and Lezajlott means the confirmed Időpont has ended (or started, if it has no end).
_Avoid_: Szavazás (that word belongs to food polls), open/confirmed/cancelled/done, pending, closed

**Időpont**:
One candidate time proposed for an Esemény. Exactly one becomes the Esemény's confirmed time.
_Avoid_: Slot, option, time

**Visszajelzés**:
An answer to one Időpont (igen, talán or nem), given by a Felhasználó, or by a Tag for a name-only Vendég they added.
_Avoid_: Vote, RSVP, response, Szavazat

**Helyezés**:
A place in a ranking (1., 2., …), used both for the Időpontok of an Esemény (counting only Tags' Visszajelzések) and for the Javaslatok of an Étkezés. Tied entries share a place, and an entry nobody has answered has none.
_Avoid_: Éllovas, leader, rank, score

**Résztvevő**:
A Tag or Vendég (including a name-only Vendég) whose Visszajelzés on the confirmed Időpont is igen. An Esemény has no Résztvevők until it is Véglegesítve.
_Avoid_: Attendee, participant, igen-voter

**Helyszín**:
Where an Esemény takes place: a free-text name with an optional Google Maps link. Each Esemény has one.
_Avoid_: Location, venue, place

**Ráérés**:
A Felhasználó's day-by-day statement of whether they are free, busy or unknown. It is shared across all their Csoports and is used for planning, never as an answer to an Időpont.
_Avoid_: Availability, Elérhetőség, schedule, free time

### Kiegészítők

**Kiegészítő**:
An optional add-on the Szervező switches on for one Esemény: Költségek, Autók, Szállás, Felszerelés, Kaja or Lejátszási lista. It can be set up during Egyeztetés, and Résztvevők can claim things in it once the Esemény is Véglegesítve.
_Avoid_: Module, plugin, feature, add-on

**Költségek** / **Kiadás**:
Költségek is the Kiegészítő for shared money. A Kiadás is one cost in it, with a payer, an amount in HUF, and the Résztvevők it is split among. A Kiadás can be planned before it is paid.
_Avoid_: Expense, bill, cost, költség (for a single item)

**Elszámolás**:
The one-time calculation, after an Esemény is Lezajlott, of the fewest transfers that settle everyone's Kiadások. It runs once every Résztvevő has confirmed their Kiadások are recorded, or once the Szervező closes it without them. After that, the Kiadások are locked, unless the Szervező reopens it before any transfer is marked done.
_Avoid_: Settlement, settle-up, balance, Törlesztés

**Autók** / **Autó** / **Sofőr**:
Autók is the Kiegészítő for getting there. An Autó is a car offered by a Résztvevő, its Sofőr, with a number of free seats that other Résztvevők claim.
_Avoid_: Car pool, ride, driver

**Saját úton**:
A Résztvevő's mark that they are getting there without an Autó.
_Avoid_: Own transport, not needed

**Szállás** / **Szoba** / **Ágy**:
Szállás is the Kiegészítő for sleeping at one place. It is made of Szobák (rooms), each with Ágyak (beds), and each Ágy sleeps one or two people. Résztvevők claim a spot in an Ágy.
_Avoid_: Accommodation, lodging, room, bed

**Máshol alszik**:
A Résztvevő's mark that they are sleeping somewhere other than the Szállás.
_Avoid_: Not staying, own accommodation

**Felszerelés** / **Tétel**:
Felszerelés is the Kiegészítő for things to bring. A Tétel is one thing on the list, with a needed quantity that Résztvevők claim, partially if they like. A Tétel marked "mindenki hozza" is a reminder for everyone rather than something to claim.
_Avoid_: Inventory, gear, item, packing list

**Vállalás**:
A Résztvevő's claim of part of a Tétel's quantity ("Anna: 2 of 3"), or a Tag's for their name-only Vendég. It is released when they drop out.
_Avoid_: Claim (for a Tétel), pledge, reservation

**Kaja** / **Étkezés** / **Javaslat** / **Szavazás**:
Kaja is the Kiegészítő for deciding what to eat. An Étkezés is one meal, a Javaslat is a dish suggested for it, and the Szavazás is the approval vote on its Javaslatok, from which the Szervező picks the winner.
_Avoid_: Food poll, meal vote, suggestion, Egyeztetés (that word is the Esemény state)

**Lejátszási lista**:
The Kiegészítő that gives an Esemény a shared Spotify playlist, to which Résztvevők add songs.
_Avoid_: Playlist, Spotify, zene (as a noun for the feature)
