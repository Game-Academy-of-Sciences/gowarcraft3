// Author:  Niels A.D.
// Project: gowarcraft3 (https://github.com/nielsAD/gowarcraft3)
// License: Mozilla Public License, v2.0

package w3g

import (
	"io"
	"strings"
	"unicode"

	"github.com/nielsAD/gowarcraft3/protocol"
	"github.com/nielsAD/gowarcraft3/protocol/w3gs"
)

// Record interface.
type Record interface {
	Serialize(buf *protocol.Buffer, enc *Encoding) error
	Deserialize(buf *protocol.Buffer, enc *Encoding) error
}

// Encoding options for (de)serialization
type Encoding struct {
	w3gs.Encoding
}

// DefaultFactory maps record ID to matching type
var DefaultFactory = MapFactory{
	RidGameInfo:       func(_ *Encoding) Record { return &GameInfo{} },
	RidPlayerInfo:     func(_ *Encoding) Record { return &PlayerInfo{} },
	RidPlayerLeft:     func(_ *Encoding) Record { return &PlayerLeft{} },
	RidSlotInfo:       func(_ *Encoding) Record { return &SlotInfo{} },
	RidCountDownStart: func(_ *Encoding) Record { return &CountDownStart{} },
	RidCountDownEnd:   func(_ *Encoding) Record { return &CountDownEnd{} },
	RidGameStart:      func(_ *Encoding) Record { return &GameStart{} },
	RidTimeSlot2:      func(_ *Encoding) Record { return &TimeSlot{} },
	RidTimeSlot:       func(_ *Encoding) Record { return &TimeSlot{} },
	RidChatMessage: func(e *Encoding) Record {
		if e.GameVersion == 0 || e.GameVersion > 2 {
			return &ChatMessage{}
		}
		return &TimeSlotAck{}
	},
	RidTimeSlotAck: func(_ *Encoding) Record { return &TimeSlotAck{} },
	RidDesync:      func(_ *Encoding) Record { return &Desync{} },
	RidEndTimer:    func(_ *Encoding) Record { return &EndTimer{} },
	RidPlayerExtra: func(_ *Encoding) Record { return &PlayerExtra{} },
}

// GameInfo record [0x10]
//
// Format:
//
//       Size   | Name
//   -----------+--------------------------
//       4 byte | Number of host records
//     variable | PlayerInfo for host
//     variable | GameName (null terminated string)
//       1 byte | Nullbyte
//     variable | Encoded String (null terminated)
//              |  - GameSettings
//              |  - Map&CreatorName
//       4 byte | PlayerCount
//       4 byte | GameType
//       4 byte | LanguageID
//
type GameInfo struct {
	HostPlayer   PlayerInfo
	GameName     string
	GameSettings w3gs.GameSettings
	GameFlags    w3gs.GameFlags
	NumSlots     uint32
	LanguageID   uint32
}

// Serialize encodes the struct into its binary form.
func (rec *GameInfo) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	buf.WriteUInt8(RidGameInfo)

	buf.WriteUInt32(1)
	rec.HostPlayer.SerializeContent(buf, enc)

	buf.WriteCString(rec.GameName)
	buf.WriteUInt8(0)

	rec.GameSettings.SerializeContent(buf, &enc.Encoding)
	buf.WriteUInt32(rec.NumSlots)
	buf.WriteUInt32(uint32(rec.GameFlags))
	buf.WriteUInt32(rec.LanguageID)

	return nil
}

// Deserialize decodes the binary data generated by Serialize.
func (rec *GameInfo) Deserialize(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 24 {
		return io.ErrShortBuffer
	}

	// Skip record ID
	buf.Skip(1)

	if buf.ReadUInt32() != 1 {
		return ErrUnexpectedConst
	}

	if err := rec.HostPlayer.DeserializeContent(buf, enc); err != nil {
		return err
	}

	if buf.Size() < 15 {
		return io.ErrShortBuffer
	}

	var err error
	if rec.GameName, err = buf.ReadCString(); err != nil {
		return err
	}

	if buf.Size() < 14 {
		return io.ErrShortBuffer
	}
	if buf.ReadUInt8() != 0 {
		return ErrUnexpectedConst
	}

	if err := rec.GameSettings.DeserializeContent(buf, &enc.Encoding); err != nil {
		return err
	}

	if buf.Size() < 12 {
		return io.ErrShortBuffer
	}

	rec.NumSlots = buf.ReadUInt32()
	rec.GameFlags = w3gs.GameFlags(buf.ReadUInt32())
	rec.LanguageID = buf.ReadUInt32()

	return nil
}

// PlayerInfo record [0x16]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//     1 byte   | PlayerID
//     n bytes  | PlayerName (null terminated string)
//     1 byte   | size of additional data:
//              |  0x01 = custom
//              |  0x08 = ladder
//
//   * If custom (0x01):
//       1 byte    | null byte (1 byte)
//   * If ladder (0x08):
//       4 bytes   | runtime of player's Warcraft.exe in milliseconds
//       4 bytes   | player race flags:
//                 |   0x01=human
//                 |   0x02=orc
//                 |   0x04=nightelf
//                 |   0x08=undead
//                 |  (0x10=daemon)
//                 |   0x20=random
//                 |   0x40=race selectable/fixed
//
type PlayerInfo struct {
	ID          uint8
	Name        string
	Race        w3gs.RacePref
	JoinCounter uint32
}

// Serialize encodes the struct into its binary form.
func (rec *PlayerInfo) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	buf.WriteUInt8(RidPlayerInfo)
	rec.SerializeContent(buf, enc)
	buf.WriteUInt32(0)
	return nil
}

// Deserialize decodes the binary data generated by Serialize.
func (rec *PlayerInfo) Deserialize(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 9 {
		return io.ErrShortBuffer
	}

	// Skip record ID
	buf.Skip(1)

	if err := rec.DeserializeContent(buf, enc); err != nil {
		return err
	}

	// Skip unknown
	if buf.Size() < 4 {
		return io.ErrShortBuffer
	}
	buf.Skip(4)

	return nil
}

// SerializeContent encodes the struct into its binary form without record ID.
func (rec *PlayerInfo) SerializeContent(buf *protocol.Buffer, enc *Encoding) {
	buf.WriteUInt8(rec.ID)
	buf.WriteCString(rec.Name)

	if rec.JoinCounter == 0 && rec.Race == 0 {
		buf.WriteUInt8(1)
		buf.WriteUInt8(0)
	} else {
		buf.WriteUInt8(8)
		buf.WriteUInt32(rec.JoinCounter)
		buf.WriteUInt32(uint32(rec.Race))
	}
}

// DeserializeContent decodes the binary data generated by SerializeContent.
func (rec *PlayerInfo) DeserializeContent(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 4 {
		return io.ErrShortBuffer
	}

	rec.ID = buf.ReadUInt8()

	var err error
	if rec.Name, err = buf.ReadCString(); err != nil {
		return err
	}

	if buf.Size() < 2 {
		return io.ErrShortBuffer
	}

	var len = buf.ReadUInt8()
	if buf.Size() < int(len) {
		return io.ErrShortBuffer
	}

	switch len {
	case 0x01, 0x02:
		buf.Skip(int(len))
		fallthrough
	case 0x00:
		rec.JoinCounter = 0
		rec.Race = 0
	case 0x08:
		rec.JoinCounter = buf.ReadUInt32()
		rec.Race = w3gs.RacePref(buf.ReadUInt32())
	default:
		return ErrUnexpectedConst
	}

	return nil
}

// PlayerLeft record [0x17]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//      1 dword | reason
//              |  0x01 - connection closed by remote game
//              |  0x0C - connection closed by local game
//              |  0x0E - unknown (rare) (almost like 0x01)
//      1 byte  | PlayerID
//      1 dword | result - see table below
//      1 dword | unknown (number of replays saved this warcraft session?)
//
type PlayerLeft struct {
	Local    bool
	PlayerID uint8
	Reason   w3gs.LeaveReason
	Counter  uint32
}

// Serialize encodes the struct into its binary form.
func (rec *PlayerLeft) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	buf.WriteUInt8(RidPlayerLeft)
	if rec.Local {
		buf.WriteUInt32(0x0C)
	} else {
		buf.WriteUInt32(0x01)
	}
	buf.WriteUInt8(rec.PlayerID)
	buf.WriteUInt32(uint32(rec.Reason))
	buf.WriteUInt32(rec.Counter)
	return nil
}

// Deserialize decodes the binary data generated by Serialize.
func (rec *PlayerLeft) Deserialize(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 14 {
		return io.ErrShortBuffer
	}

	// Skip record ID
	buf.Skip(1)

	switch buf.ReadUInt32() {
	case 0x01, 0x0E:
		rec.Local = false
	case 0x0C:
		rec.Local = true
	default:
		return ErrUnexpectedConst
	}

	rec.PlayerID = buf.ReadUInt8()
	rec.Reason = w3gs.LeaveReason(buf.ReadUInt32())
	rec.Counter = buf.ReadUInt32()

	return nil
}

// SlotInfo record [0x19]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//     1 word   | number of data bytes following
//     1 byte   | nr of SlotRecords following (== nr of slots on startscreen)
//     n bytes  | nr * SlotRecord
//     1 dword  | RandomSeed
//     1 byte   | SelectMode
//              |   0x00 - team & race selectable (for standard custom games)
//              |   0x01 - team not selectable
//              |          (map setting: fixed alliances in WorldEditor)
//              |   0x03 - team & race not selectable
//              |          (map setting: fixed player properties in WorldEditor)
//              |   0x04 - race fixed to random
//              |          (extended map options: random races selected)
//              |   0xcc - Automated Match Making (ladder)
//     1 byte   | StartSpotCount (nr. of start positions in map)
//
//   For each slot:
//     1 byte   | player id (0x00 for computer players)
//     1 byte   | map download percent: 0x64 in custom, 0xff in ladder
//     1 byte   | slotstatus:
//              |   0x00 empty slot
//              |   0x01 closed slot
//              |   0x02 used slot
//     1 byte   | computer player flag:
//              |   0x00 for human player
//              |   0x01 for computer player
//     1 byte   | team number:0 - 11
//              | (team 12 == observer or referee)
//     1 byte   | color (0-11):
//              |   value+1 matches player colors in world editor:
//              |   (red, blue, cyan, purple, yellow, orange, green,
//              |    pink, gray, light blue, dark green, brown)
//              |   color 12 == observer or referee
//     1 byte   | player race flags (as selected on map screen):
//              |   0x01=human
//              |   0x02=orc
//              |   0x04=nightelf
//              |   0x08=undead
//              |   0x20=random
//              |   0x40=race selectable/fixed
//     1 byte   | computer AI strength: (only present in v1.03 or higher)
//              |   0x00 for easy
//              |   0x01 for normal
//              |   0x02 for insane
//              | for non-AI players this seems to be always 0x01
//     1 byte   | player handicap in percent (as displayed on startscreen)
//              | valid values: 0x32, 0x3C, 0x46, 0x50, 0x5A, 0x64
//              | (field only present in v1.07 or higher)
//
type SlotInfo struct {
	w3gs.SlotInfo
}

// Serialize encodes the struct into its binary form.
func (rec *SlotInfo) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	buf.WriteUInt8(RidSlotInfo)
	rec.SerializeContent(buf, &enc.Encoding)
	return nil
}

// Deserialize decodes the binary data generated by Serialize.
func (rec *SlotInfo) Deserialize(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 10 {
		return io.ErrShortBuffer
	}

	// Skip record ID
	buf.Skip(1)

	return rec.DeserializeContent(buf, &enc.Encoding)
}

// GameStart record [0x1C]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//      1 dword | unknown (always 0x01 so far)
//
type GameStart struct{}

// Serialize encodes the struct into its binary form.
func (rec *GameStart) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	buf.WriteUInt8(RidGameStart)
	buf.WriteUInt32(0x01)
	return nil
}

// Deserialize decodes the binary data generated by Serialize.
func (rec *GameStart) Deserialize(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 5 {
		return io.ErrShortBuffer
	}

	// Skip record ID
	buf.Skip(1)

	if buf.ReadUInt32() != 0x01 {
		return ErrUnexpectedConst
	}

	return nil
}

// CountDownStart record [0x1A]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//      1 dword | unknown (always 0x01 so far)
//
type CountDownStart struct {
	GameStart
}

// Serialize encodes the struct into its binary form.
func (rec *CountDownStart) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	buf.WriteUInt8(RidCountDownStart)
	buf.WriteUInt32(0x01)
	return nil
}

// CountDownEnd record [0x1B]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//      1 dword | unknown (always 0x01 so far)
//
type CountDownEnd struct {
	GameStart
}

// Serialize encodes the struct into its binary form.
func (rec *CountDownEnd) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	buf.WriteUInt8(RidCountDownEnd)
	buf.WriteUInt32(0x01)
	return nil
}

// TimeSlot record [0x1E] / [0x1F]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//     1 word   | n = number of bytes that follow
//     1 word   | time increment (milliseconds)
//              |   about 250 ms in battle.net
//              |   about 100 ms in LAN and single player
//     n-2 byte | CommandData block(s) (not present if n=2)
//
type TimeSlot struct {
	w3gs.TimeSlot
}

// Serialize encodes the struct into its binary form.
func (rec *TimeSlot) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	if rec.Fragment || (enc.GameVersion > 0 && enc.GameVersion <= 2) {
		buf.WriteUInt8(RidTimeSlot2)
	} else {
		buf.WriteUInt8(RidTimeSlot)
	}

	// Placeholder for size
	buf.WriteUInt16(0)
	var start = buf.Size()

	buf.WriteUInt16(rec.TimeIncrementMS)

	for i := 0; i < len(rec.Actions); i++ {
		buf.WriteUInt8(rec.Actions[i].PlayerID)
		buf.WriteUInt16(uint16(len(rec.Actions[i].Data)))
		buf.WriteBlob(rec.Actions[i].Data)
	}

	// Set size
	buf.WriteUInt16At(start-2, uint16(buf.Size()-start))

	return nil
}

// Deserialize decodes the binary data generated by Serialize.
func (rec *TimeSlot) Deserialize(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 5 {
		return io.ErrShortBuffer
	}

	rec.Fragment = buf.ReadUInt8() == RidTimeSlot2 && (enc.GameVersion == 0 || enc.GameVersion > 2)

	var size = int(buf.ReadUInt16())
	if size < 2 || buf.Size() < size {
		return io.ErrShortBuffer
	}

	rec.TimeIncrementMS = buf.ReadUInt16()
	size -= 2

	var i = 0

	rec.Actions = rec.Actions[:0]
	for size >= 3 {
		if cap(rec.Actions) < i+1 {
			rec.Actions = append(rec.Actions, w3gs.PlayerAction{})
		} else {
			rec.Actions = rec.Actions[:i+1]
		}

		rec.Actions[i].PlayerID = buf.ReadUInt8()

		var subsize = int(buf.ReadUInt16())
		if size < subsize {
			return ErrBadFormat
		}
		size -= 3 + subsize

		rec.Actions[i].Data = append(rec.Actions[i].Data[:0], buf.ReadBlob(subsize)...)
		i++
	}

	if size != 0 {
		return ErrBadFormat
	}

	return nil
}

// ChatMessage record [0x20]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//     1 byte   | PlayerID (message sender)
//     1 word   | n = number of bytes that follow
//     1 byte   | flags
//              |   0x10   for delayed startup screen messages
//              |   0x20   for normal messages
//     1 dword  | chat mode (not present if flag = 0x10):
//              |   0x00   for messages to all players
//              |   0x01   for messages to allies
//              |   0x02   for messages to observers or referees
//              |   0x03+N for messages to specific player N (with N = slotnumber)
//     n bytes  | zero terminated string containing the text message
//
type ChatMessage struct {
	w3gs.Message
}

// Serialize encodes the struct into its binary form.
func (rec *ChatMessage) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	buf.WriteUInt8(RidChatMessage)
	buf.WriteUInt8(rec.SenderID)

	switch rec.Type {
	case w3gs.MsgChatExtra:
		buf.WriteUInt16(uint16(6 + len(rec.Content)))
	case w3gs.MsgChat:
		buf.WriteUInt16(uint16(2 + len(rec.Content)))
	default:
		buf.WriteUInt16(2)
	}

	buf.WriteUInt8(uint8(rec.Type))

	switch rec.Type {
	case w3gs.MsgChatExtra:
		buf.WriteUInt32(uint32(rec.Scope))
		fallthrough
	case w3gs.MsgChat:
		buf.WriteCString(rec.Content)
	default:
		buf.WriteUInt8(rec.NewVal)
	}

	return nil
}

// Deserialize decodes the binary data generated by Serialize.
func (rec *ChatMessage) Deserialize(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 3 {
		return io.ErrShortBuffer
	}

	// Skip record ID
	buf.Skip(1)

	rec.RecipientIDs = nil
	rec.SenderID = buf.ReadUInt8()
	rec.Scope = w3gs.ScopeAll
	rec.NewVal = 0
	rec.Content = ""

	var size = int(buf.ReadUInt16())
	if size < 2 || buf.Size() < size {
		return io.ErrShortBuffer
	}

	rec.Type = w3gs.MessageType(buf.ReadUInt8())

	switch rec.Type {
	case w3gs.MsgChatExtra:
		if size < 6 {
			return ErrBadFormat
		}
		size -= 4
		rec.Scope = w3gs.MessageScope(buf.ReadUInt32())
		fallthrough
	case w3gs.MsgChat:
		var err error
		if rec.Content, err = buf.ReadCString(); err != nil {
			return err
		}

		// Parse extra strings (nwg quirk)
		size -= 2 + len(rec.Content)
		for size > 0 {
			buf, err := buf.ReadCString()
			if err != nil {
				return err
			}

			if strings.IndexFunc(buf, func(r rune) bool { return !unicode.IsPrint(r) }) != -1 {
				return ErrBadFormat
			}

			rec.Content += buf
			size -= len(buf) + 1
		}

		if size != 0 {
			return ErrBadFormat
		}
	default:
		if size != 2 {
			return ErrBadFormat
		}
		rec.NewVal = buf.ReadUInt8()
	}

	return nil
}

// TimeSlotAck record [0x22]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//      1 byte  | number of bytes following (always 0x04 so far)
//      1 dword | checksum
//
type TimeSlotAck struct {
	Checksum []byte
}

// Serialize encodes the struct into its binary form.
func (rec *TimeSlotAck) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	if enc.GameVersion == 0 || enc.GameVersion > 2 {
		buf.WriteUInt8(RidTimeSlotAck)
	} else {
		buf.WriteUInt8(RidChatMessage)
	}

	buf.WriteUInt8(uint8(len(rec.Checksum)))
	buf.WriteBlob(rec.Checksum)
	return nil
}

// Deserialize decodes the binary data generated by Serialize.
func (rec *TimeSlotAck) Deserialize(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 2 {
		return io.ErrShortBuffer
	}

	// Skip record ID
	buf.Skip(1)

	var size = int(buf.ReadUInt8())
	if buf.Size() < size {
		return io.ErrShortBuffer
	}

	rec.Checksum = append(rec.Checksum[:0], buf.ReadBlob(size)...)
	return nil
}

// Desync record [0x23]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//      1 dword | unknown
//      1 byte  | unknown (always 4?)
//      1 dword | unknown (random?)
//      1 byte  | unknown (always 0?)
//
type Desync struct {
	w3gs.Desync
}

// Serialize encodes the struct into its binary form.
func (rec *Desync) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	buf.WriteUInt8(RidDesync)
	rec.SerializeContent(buf, &enc.Encoding)
	return nil
}

// Deserialize decodes the binary data generated by Serialize.
func (rec *Desync) Deserialize(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 11 {
		return io.ErrShortBuffer
	}

	// Skip record ID
	buf.Skip(1)

	return rec.DeserializeContent(buf, &enc.Encoding)
}

// EndTimer record [0x2F]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//      1 dword | mode:
//              |   0x00 countdown is running
//              |   0x01 countdown is over (end is forced *now*)
//      1 dword | countdown time in sec
//
type EndTimer struct {
	GameOver     bool
	CountDownSec uint32
}

// Serialize encodes the struct into its binary form.
func (rec *EndTimer) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	buf.WriteUInt8(RidEndTimer)
	buf.WriteBool32(rec.GameOver)
	buf.WriteUInt32(rec.CountDownSec)
	return nil
}

// Deserialize decodes the binary data generated by Serialize.
func (rec *EndTimer) Deserialize(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 9 {
		return io.ErrShortBuffer
	}

	// Skip record ID
	buf.Skip(1)

	rec.GameOver = buf.ReadBool32()
	rec.CountDownSec = buf.ReadUInt32()

	return nil
}

// PlayerExtra record [0x39]
//
// Format:
//
//    size/type | Description
//   -----------+-----------------------------------------------------------
//      1 byte  | sub type (0x03)
//              |   0x03   battle.net profile data
//              |   0x04   in-game skins
//      1 dword | number of bytes following
//      n bytes | protobuf encoded struct
//
//   For each battle.net profile (sub type 0x03, encoded with protobuf):
//      1 byte  | player ID
//      string  | battletag
//      string  | clan
//      string  | portrait
//      1 byte  | team
//      string  | unknown
//
//   For each player (sub type 0x04, encoded with protobuf):
//      1 byte  | player ID
//      For each in-game skin:
//      qword   | unit ID
//      qword   | skin ID
//      string  | skin collection
//
type PlayerExtra struct {
	w3gs.PlayerExtra
}

// Serialize encodes the struct into its binary form.
func (rec *PlayerExtra) Serialize(buf *protocol.Buffer, enc *Encoding) error {
	buf.WriteUInt8(RidPlayerExtra)
	return rec.PlayerExtra.SerializeContent(buf, &enc.Encoding)
}

// Deserialize decodes the binary data generated by Serialize.
func (rec *PlayerExtra) Deserialize(buf *protocol.Buffer, enc *Encoding) error {
	if buf.Size() < 6 {
		return io.ErrShortBuffer
	}

	// Skip record ID
	buf.Skip(1)

	return rec.PlayerExtra.DeserializeContent(buf, &enc.Encoding)
}
