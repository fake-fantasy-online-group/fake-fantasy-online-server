package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
)

var (
	ErrMailRecipientNotFound = errors.New("mail: recipient not found")
	ErrMailSelf              = errors.New("mail: cannot send to self")
	ErrMailboxFull           = errors.New("mail: mailbox full")
	ErrMailNotFound          = errors.New("mail: not found")
	ErrMailNothingToClaim    = errors.New("mail: nothing to claim")
	ErrMailCannotDelete      = errors.New("mail: unclaimed assets")
	ErrMailCannotReturn      = errors.New("mail: cannot return")
)

func (p *Postgres) LoadMails(ctx context.Context, recipientID int64, rule domain.MailRule) ([]domain.Mail, error) {
	if !rule.Valid() {
		return nil, fmt.Errorf("mail: invalid rule")
	}
	if err := p.expireMails(ctx, recipientID, rule); err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, `
		SELECT id,recipient_id,sender_id,sender_name,title,body,money,caiyu,mail_type,
		       is_read,returned,created_at,expires_at
		  FROM character_mails
		 WHERE recipient_id=$1
		 ORDER BY created_at DESC,id DESC
		 LIMIT $2`, recipientID, rule.Capacity)
	if err != nil {
		return nil, fmt.Errorf("mail: list: %w", err)
	}
	defer rows.Close()
	var out []domain.Mail
	for rows.Next() {
		var m domain.Mail
		var typ int16
		if err := rows.Scan(&m.ID, &m.RecipientID, &m.SenderID, &m.SenderName, &m.Title,
			&m.Body, &m.Money, &m.Caiyu, &typ, &m.Read, &m.Returned,
			&m.CreatedAt, &m.ExpiresAt); err != nil {
			return nil, fmt.Errorf("mail: scan: %w", err)
		}
		m.Type = uint8(typ)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mail: rows: %w", err)
	}
	for i := range out {
		attachments, err := p.loadMailAttachments(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Attachments = attachments
	}
	return out, nil
}

func (p *Postgres) expireMails(ctx context.Context, recipientID int64, rule domain.MailRule) error {
	return p.WithTx(ctx, func(raw Store) error {
		tx := raw.(*pgTx)
		type expiredMail struct {
			id, senderID, money, caiyu int64
			title, body                string
			typ                        int16
			returned                   bool
		}
		rows, err := tx.tx.Query(ctx, `
			SELECT id,sender_id,title,body,money,caiyu,mail_type,returned
			  FROM character_mails
			 WHERE recipient_id=$1 AND expires_at<=NOW()
			 FOR UPDATE`, recipientID)
		if err != nil {
			return err
		}
		var expired []expiredMail
		for rows.Next() {
			var m expiredMail
			if err := rows.Scan(&m.id, &m.senderID, &m.title, &m.body, &m.money,
				&m.caiyu, &m.typ, &m.returned); err != nil {
				rows.Close()
				return err
			}
			expired = append(expired, m)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		var recipientName string
		if len(expired) > 0 {
			if err := tx.tx.QueryRow(ctx, `SELECT name FROM characters WHERE id=$1`, recipientID).Scan(&recipientName); err != nil {
				return err
			}
		}
		for _, m := range expired {
			var attachments int
			if err := tx.tx.QueryRow(ctx, `SELECT count(*) FROM character_mail_attachments WHERE mail_id=$1`, m.id).Scan(&attachments); err != nil {
				return err
			}
			hasAssets := m.money > 0 || m.caiyu > 0 || attachments > 0
			if !hasAssets || m.returned || m.senderID <= 0 {
				if _, err := tx.tx.Exec(ctx, `DELETE FROM character_mails WHERE id=$1`, m.id); err != nil {
					return err
				}
				continue
			}
			var exists bool
			var inbox int32
			if err := tx.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM characters WHERE id=$1)`, m.senderID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				if _, err := tx.tx.Exec(ctx, `DELETE FROM character_mails WHERE id=$1`, m.id); err != nil {
					return err
				}
				continue
			}
			if err := tx.tx.QueryRow(ctx, `SELECT count(*) FROM character_mails WHERE recipient_id=$1`, m.senderID).Scan(&inbox); err != nil {
				return err
			}
			if inbox >= rule.Capacity {
				continue // 发件箱暂满时保留原邮件，避免吞掉托管物权。
			}
			var returnedID int64
			expires := time.Now().AddDate(0, 0, int(rule.ExpireDays))
			if err := tx.tx.QueryRow(ctx, `INSERT INTO character_mails(recipient_id,sender_id,sender_name,title,body,money,caiyu,mail_type,is_read,returned,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,FALSE,TRUE,$9) RETURNING id`,
				m.senderID, recipientID, recipientName, "退回："+m.title, m.body,
				m.money, m.caiyu, m.typ, expires).Scan(&returnedID); err != nil {
				return err
			}
			if _, err := tx.tx.Exec(ctx, `UPDATE character_mail_attachments SET mail_id=$1 WHERE mail_id=$2`, returnedID, m.id); err != nil {
				return err
			}
			if _, err := tx.tx.Exec(ctx, `DELETE FROM character_mails WHERE id=$1`, m.id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *Postgres) loadMailAttachments(ctx context.Context, mailID int64) ([]domain.MailAttachment, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT ordinal,uid,item_id,count,durability,max_durability,durability_wear_raw,
		       bound,locked,refine_level,socket_count,sockets,wash_quality,wash_count,
		       wash_attrs,wash_values,wash_modes,fused_appearance_item_id
		  FROM character_mail_attachments WHERE mail_id=$1 ORDER BY ordinal`, mailID)
	if err != nil {
		return nil, fmt.Errorf("mail: list attachments: %w", err)
	}
	defer rows.Close()
	var out []domain.MailAttachment
	var instanceUIDs []int64
	for rows.Next() {
		var a domain.MailAttachment
		var item int32
		var socketCount, washQuality, washCount int16
		var sockets, wa, wv, wm []int32
		if err := rows.Scan(&a.Ordinal, &a.Stack.UID, &item, &a.Stack.Count,
			&a.Stack.Durability, &a.Stack.MaxDurability, &a.Stack.DurabilityWearRaw,
			&a.Stack.Bound, &a.Stack.Locked, &a.Stack.RefineLevel, &socketCount, &sockets,
			&washQuality, &washCount, &wa, &wv, &wm, &a.Stack.FusedAppearance); err != nil {
			return nil, fmt.Errorf("mail: scan attachment: %w", err)
		}
		a.Stack.Item = domain.ItemID(item)
		a.Stack.SocketCount = uint8(socketCount)
		for j := range a.Stack.Sockets {
			if j < len(sockets) {
				a.Stack.Sockets[j] = domain.ItemID(sockets[j])
			}
		}
		loadWashArrays(&a.Stack, washQuality, washCount, wa, wv, wm)
		if a.Stack.UID > 0 {
			instanceUIDs = append(instanceUIDs, a.Stack.UID)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	facts, err := loadItemInstances(ctx, p.pool, instanceUIDs)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Stack.UID <= 0 {
			continue
		}
		out[i].Stack, err = hydrateStack(out[i].Stack, facts)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (p *Postgres) SendMail(ctx context.Context, sender domain.Snapshot, draft domain.MailDraft, rule domain.MailRule) (int64, error) {
	if sender.Char == nil || !rule.Valid() {
		return 0, fmt.Errorf("mail: invalid send")
	}
	var mailID int64
	err := p.WithTx(ctx, func(raw Store) error {
		tx := raw.(*pgTx)
		recipient, err := tx.CharByName(ctx, draft.RecipientName)
		if err != nil {
			return ErrMailRecipientNotFound
		}
		if recipient.ID == sender.Char.ID {
			return ErrMailSelf
		}
		var count int32
		if err := tx.tx.QueryRow(ctx, `SELECT count(*) FROM character_mails WHERE recipient_id=$1`, recipient.ID).Scan(&count); err != nil {
			return err
		}
		if count >= rule.Capacity {
			return ErrMailboxFull
		}
		if err := tx.SaveSnapshot(ctx, sender); err != nil {
			return err
		}
		expires := time.Now().AddDate(0, 0, int(rule.ExpireDays))
		if err := tx.tx.QueryRow(ctx, `
			INSERT INTO character_mails(recipient_id,sender_id,sender_name,title,body,money,caiyu,
			 mail_type,is_read,returned,expires_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,FALSE,FALSE,$9) RETURNING id`,
			recipient.ID, sender.Char.ID, sender.Char.Name, draft.Title, draft.Body,
			draft.Money, draft.Caiyu, int16(draft.Type), expires).Scan(&mailID); err != nil {
			return err
		}
		return insertMailAttachments(ctx, tx.tx, mailID, draft.Attachments)
	})
	return mailID, err
}

func insertMailAttachments(ctx context.Context, tx pgx.Tx, mailID int64, attachments []domain.MailAttachment) error {
	instances := make([]domain.Stack, 0, len(attachments))
	for _, attachment := range attachments {
		instances = append(instances, attachment.Stack)
	}
	if err := saveItemInstances(ctx, tx, "mail", instances...); err != nil {
		return err
	}
	for i, a := range attachments {
		s := a.Stack
		wa, wv, wm := washArraysOf(s)
		if _, err := tx.Exec(ctx, `
			INSERT INTO character_mail_attachments(mail_id,ordinal,uid,item_id,count,durability,
			 max_durability,durability_wear_raw,bound,locked,refine_level,socket_count,sockets,
			 wash_quality,wash_count,wash_attrs,wash_values,wash_modes,fused_appearance_item_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
			mailID, i, s.UID, int32(s.Item), s.Count, s.Durability, s.MaxDurability,
			s.DurabilityWearRaw, s.Bound, s.Locked, s.RefineLevel, int16(s.SocketCount),
			socketIDsOf(s), int16(s.WashQuality), int16(s.WashCount), wa, wv, wm,
			int32(s.FusedAppearance)); err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) ClaimMail(ctx context.Context, recipient domain.Snapshot, mailID int64) error {
	if recipient.Char == nil {
		return ErrMailNotFound
	}
	return p.WithTx(ctx, func(raw Store) error {
		tx := raw.(*pgTx)
		var money, caiyu int64
		if err := tx.tx.QueryRow(ctx, `SELECT money,caiyu FROM character_mails WHERE id=$1 AND recipient_id=$2 FOR UPDATE`,
			mailID, recipient.Char.ID).Scan(&money, &caiyu); err != nil {
			return ErrMailNotFound
		}
		var n int
		if err := tx.tx.QueryRow(ctx, `SELECT count(*) FROM character_mail_attachments WHERE mail_id=$1`, mailID).Scan(&n); err != nil {
			return err
		}
		if money == 0 && caiyu == 0 && n == 0 {
			return ErrMailNothingToClaim
		}
		if err := tx.SaveSnapshot(ctx, recipient); err != nil {
			return err
		}
		if _, err := tx.tx.Exec(ctx, `DELETE FROM character_mail_attachments WHERE mail_id=$1`, mailID); err != nil {
			return err
		}
		_, err := tx.tx.Exec(ctx, `UPDATE character_mails SET money=0,caiyu=0,is_read=TRUE WHERE id=$1`, mailID)
		return err
	})
}

func (p *Postgres) MarkMailRead(ctx context.Context, recipientID, mailID int64) error {
	res, err := p.pool.Exec(ctx, `UPDATE character_mails SET is_read=TRUE WHERE id=$1 AND recipient_id=$2`, mailID, recipientID)
	if err != nil || res.RowsAffected() == 0 {
		return ErrMailNotFound
	}
	return nil
}

func (p *Postgres) DeleteMail(ctx context.Context, recipientID, mailID int64) error {
	return p.WithTx(ctx, func(raw Store) error {
		tx := raw.(*pgTx)
		var money, caiyu int64
		if err := tx.tx.QueryRow(ctx, `SELECT money,caiyu FROM character_mails WHERE id=$1 AND recipient_id=$2 FOR UPDATE`, mailID, recipientID).Scan(&money, &caiyu); err != nil {
			return ErrMailNotFound
		}
		var n int
		if err := tx.tx.QueryRow(ctx, `SELECT count(*) FROM character_mail_attachments WHERE mail_id=$1`, mailID).Scan(&n); err != nil {
			return err
		}
		if money != 0 || caiyu != 0 || n != 0 {
			return ErrMailCannotDelete
		}
		_, err := tx.tx.Exec(ctx, `DELETE FROM character_mails WHERE id=$1`, mailID)
		return err
	})
}

func (p *Postgres) ReturnMail(ctx context.Context, recipientID, mailID int64, rule domain.MailRule) error {
	return p.WithTx(ctx, func(raw Store) error {
		tx := raw.(*pgTx)
		var senderID, money, caiyu int64
		var senderName, title, body string
		var typ int16
		var returned bool
		if err := tx.tx.QueryRow(ctx, `SELECT sender_id,sender_name,title,body,money,caiyu,mail_type,returned FROM character_mails WHERE id=$1 AND recipient_id=$2 FOR UPDATE`,
			mailID, recipientID).Scan(&senderID, &senderName, &title, &body, &money, &caiyu, &typ, &returned); err != nil {
			return ErrMailNotFound
		}
		if senderID <= 0 || returned {
			return ErrMailCannotReturn
		}
		var senderExists bool
		if err := tx.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM characters WHERE id=$1)`, senderID).Scan(&senderExists); err != nil || !senderExists {
			return ErrMailCannotReturn
		}
		var count int32
		if err := tx.tx.QueryRow(ctx, `SELECT count(*) FROM character_mails WHERE recipient_id=$1`, senderID).Scan(&count); err != nil {
			return err
		}
		if count >= rule.Capacity {
			return ErrMailboxFull
		}
		var fromName string
		if err := tx.tx.QueryRow(ctx, `SELECT name FROM characters WHERE id=$1`, recipientID).Scan(&fromName); err != nil {
			return err
		}
		var returnedID int64
		expires := time.Now().AddDate(0, 0, int(rule.ExpireDays))
		if err := tx.tx.QueryRow(ctx, `INSERT INTO character_mails(recipient_id,sender_id,sender_name,title,body,money,caiyu,mail_type,is_read,returned,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,FALSE,TRUE,$9) RETURNING id`,
			senderID, recipientID, fromName, "退回："+title, body, money, caiyu, typ, expires).Scan(&returnedID); err != nil {
			return err
		}
		if _, err := tx.tx.Exec(ctx, `UPDATE character_mail_attachments SET mail_id=$1 WHERE mail_id=$2`, returnedID, mailID); err != nil {
			return err
		}
		_, err := tx.tx.Exec(ctx, `DELETE FROM character_mails WHERE id=$1`, mailID)
		return err
	})
}
