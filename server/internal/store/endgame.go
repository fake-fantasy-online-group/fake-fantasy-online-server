package store

import (
 "context"
 "errors"
 "fmt"
 "hash/fnv"
 "github.com/jackc/pgx/v5"
 "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)
var ErrEndgameRevision = errors.New("endgame: stale_revision")
var ErrEndgameReceiptConflict = errors.New("endgame: request_id_conflict")

// EndgameStore is optional: old Store implementors do not silently claim support.
// All writes, locks, and recovery reads are used within Store.WithTx.
type EndgameStore interface {
 LockEndgameCharacter(context.Context,int64) error
 LockEndgameRun(context.Context,string) error
 LoadEndgameSnapshot(context.Context,domain.Snapshot)(domain.Snapshot,error)
 LoadEndgameState(context.Context,int64)(domain.EndgameState,error)
 SaveEndgameState(context.Context,int64,domain.EndgameState,int64) error
 LoadEndgameReceipt(context.Context,int64,string)(*domain.EndgameReceipt,error)
 SaveEndgameReceipt(context.Context,domain.EndgameReceipt) error
 LoadEndgameRun(context.Context,string)(*domain.EndgameRunRecord,error)
 SaveEndgameRun(context.Context,domain.EndgameRunRecord,int64) error
}
func (p *Postgres) LockEndgameCharacter(context.Context,int64) error {return fmt.Errorf("endgame: transaction required")}
func (p *Postgres) LockEndgameRun(context.Context,string) error {return fmt.Errorf("endgame: transaction required")}
func (t *pgTx) LockEndgameCharacter(ctx context.Context,id int64) error {var found int64;return t.tx.QueryRow(ctx,`SELECT id FROM characters WHERE id=$1 FOR UPDATE`,id).Scan(&found)}
func (t *pgTx) LockEndgameRun(ctx context.Context,id string) error {h:=fnv.New64a();h.Write([]byte("daoist.endgame.run:"+id));_,err:=t.tx.Exec(ctx,`SELECT pg_advisory_xact_lock($1)`,int64(h.Sum64()));return err}
func loadEndgameState(ctx context.Context,q querier,id int64)(v domain.EndgameState,err error){err=q.QueryRow(ctx,`SELECT revision,payload::text FROM character_endgame_state WHERE char_id=$1`,id).Scan(&v.Revision,&v.Payload);if errors.Is(err,pgx.ErrNoRows){return domain.EndgameState{Payload:"{}"},nil};return}
func saveEndgameState(ctx context.Context,q querier,id int64,v domain.EndgameState,expected int64)error{
 if expected<0||v.Revision!=expected+1{return ErrEndgameRevision};var n int64
 if expected==0 {tag,e:=q.Exec(ctx,`INSERT INTO character_endgame_state(char_id,revision,payload) VALUES($1,$2,$3::jsonb) ON CONFLICT DO NOTHING`,id,v.Revision,v.Payload);if e!=nil{return e};n=tag.RowsAffected()}else{tag,e:=q.Exec(ctx,`UPDATE character_endgame_state SET revision=$2,payload=$3::jsonb WHERE char_id=$1 AND revision=$4`,id,v.Revision,v.Payload,expected);if e!=nil{return e};n=tag.RowsAffected()};if n!=1{return ErrEndgameRevision};return nil
}
func loadEndgameReceipt(ctx context.Context,q querier,id int64,key string)(*domain.EndgameReceipt,error){v:=domain.EndgameReceipt{CharID:id,RequestID:key};e:=q.QueryRow(ctx,`SELECT operation,digest,revision,response::text FROM endgame_receipts WHERE char_id=$1 AND request_id=$2`,id,key).Scan(&v.Operation,&v.Digest,&v.Revision,&v.Response);if errors.Is(e,pgx.ErrNoRows){return nil,nil};return &v,e}
func saveEndgameReceipt(ctx context.Context,q querier,v domain.EndgameReceipt)error{tag,e:=q.Exec(ctx,`INSERT INTO endgame_receipts(char_id,request_id,operation,digest,revision,response) VALUES($1,$2,$3,$4,$5,$6::jsonb) ON CONFLICT DO NOTHING`,v.CharID,v.RequestID,v.Operation,v.Digest,v.Revision,v.Response);if e!=nil{return e};if tag.RowsAffected()!=1{return ErrEndgameReceiptConflict};return nil}
func loadEndgameRun(ctx context.Context,q querier,id string)(*domain.EndgameRunRecord,error){v:=domain.EndgameRunRecord{RunID:id};e:=q.QueryRow(ctx,`SELECT revision,state,payload::text FROM endgame_runs WHERE run_id=$1`,id).Scan(&v.Revision,&v.State,&v.Payload);if errors.Is(e,pgx.ErrNoRows){return nil,nil};return &v,e}
func saveEndgameRun(ctx context.Context,q querier,v domain.EndgameRunRecord,expected int64)error{
 if expected<0||v.Revision!=expected+1{return ErrEndgameRevision};var n int64
 if expected==0{tag,e:=q.Exec(ctx,`INSERT INTO endgame_runs(run_id,revision,state,payload) VALUES($1,$2,$3,$4::jsonb) ON CONFLICT DO NOTHING`,v.RunID,v.Revision,v.State,v.Payload);if e!=nil{return e};n=tag.RowsAffected()}else{tag,e:=q.Exec(ctx,`UPDATE endgame_runs SET revision=$2,state=$3,payload=$4::jsonb WHERE run_id=$1 AND revision=$5`,v.RunID,v.Revision,v.State,v.Payload,expected);if e!=nil{return e};n=tag.RowsAffected()};if n!=1{return ErrEndgameRevision};return nil
}

func (p *Postgres) LoadEndgameState(ctx context.Context,id int64)(domain.EndgameState,error){return loadEndgameState(ctx,p.pool,id)}

func (p *Postgres) SaveEndgameState(ctx context.Context,id int64,v domain.EndgameState,expected int64)error{return saveEndgameState(ctx,p.pool,id,v,expected)}

func (p *Postgres) LoadEndgameReceipt(ctx context.Context,id int64,key string)(*domain.EndgameReceipt,error){return loadEndgameReceipt(ctx,p.pool,id,key)}

func (p *Postgres) SaveEndgameReceipt(ctx context.Context,v domain.EndgameReceipt)error{return saveEndgameReceipt(ctx,p.pool,v)}

func (p *Postgres) LoadEndgameRun(ctx context.Context,id string)(*domain.EndgameRunRecord,error){return loadEndgameRun(ctx,p.pool,id)}

func (p *Postgres) SaveEndgameRun(ctx context.Context,v domain.EndgameRunRecord,expected int64)error{return saveEndgameRun(ctx,p.pool,v,expected)}

func (t *pgTx) LoadEndgameState(ctx context.Context,id int64)(domain.EndgameState,error){return loadEndgameState(ctx,t.tx,id)}

func (t *pgTx) SaveEndgameState(ctx context.Context,id int64,v domain.EndgameState,expected int64)error{return saveEndgameState(ctx,t.tx,id,v,expected)}

func (t *pgTx) LoadEndgameReceipt(ctx context.Context,id int64,key string)(*domain.EndgameReceipt,error){return loadEndgameReceipt(ctx,t.tx,id,key)}

func (t *pgTx) SaveEndgameReceipt(ctx context.Context,v domain.EndgameReceipt)error{return saveEndgameReceipt(ctx,t.tx,v)}

func (t *pgTx) LoadEndgameRun(ctx context.Context,id string)(*domain.EndgameRunRecord,error){return loadEndgameRun(ctx,t.tx,id)}

func (t *pgTx) SaveEndgameRun(ctx context.Context,v domain.EndgameRunRecord,expected int64)error{return saveEndgameRun(ctx,t.tx,v,expected)}
