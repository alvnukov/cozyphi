// Package notify delivers desktop notifications when the agent stops or
// asks for the user's input.
//
// A Notifier is safe for concurrent use and never blocks its caller: sends
// run on a background goroutine with a hard timeout, and while one send is in
// flight the next notification is dropped instead of queueing — a stale
// notification is worthless once a newer state exists. Reconfigure swaps the
// mode and the sound mid-session, so a saved settings draft takes effect
// without restarting the process.
//
// Mode gating:
//   - ModeOff disables delivery entirely.
//   - ModeAlways delivers every notification.
//   - ModeUnfocused delivers only while the terminal reports itself unfocused.
//     Terminals without focus reporting default to focused, i.e. silent: an
//     unknown state must not turn into notification spam.
//
// Platform senders: darwin (osascript display notification) and linux
// (notify-send); both take title, body and the sound name as process
// arguments and never interpolate them into a script. Each notification
// plays a sound — DefaultSound unless the config names another or turns it
// off — so a ping is heard from another desktop, not only seen. The first
// sender failure disables the notifier for the rest of the process and is
// reported through debuglog — a missing helper binary does not heal by
// retrying every turn.
package notify
