package domain

import (
 "encoding/json"
 "errors"
 "math"
)

const (
 RiftKeyItemID ItemID = 1900028101
 RiftMaxTier int32 = 50
 RiftProgressMax int32 = 10000
 RiftNormalProgress int32 = 40
 RiftGlobeProgress int32 = 200
 RiftTimedMS int64 = 15*60*1000
 RiftHardMS int64 = 20*60*1000
 RiftReconnectMS int64 = 120000
 RiftDeathPenaltyMS int64 = 5000
)
type RiftPhase string
const ( RiftReserved RiftPhase="reserved"; RiftActive RiftPhase="active"; RiftBoss RiftPhase="boss"; RiftCompleted RiftPhase="completed"; RiftFailed RiftPhase="failed"; RiftAborted RiftPhase="aborted"; RiftClosed RiftPhase="closed" )
type TrialKeyMember struct { CharID CharID; Level, Cycle int32 }
// TrialKeyReward freezes recipient round and opening levels. One base, up to
// four peers each worth 1+1 mentorship, and round 0..5: exactly 14 maximum.
func TrialKeyReward(recipient CharID, members []TrialKeyMember) (int32,error) {
 if len(members)<1 || len(members)>5 { return 0,errors.New("invalid roster") }
 seen:=map[CharID]bool{}; var own *TrialKeyMember
 for i:=range members { m:=&members[i]; if m.CharID<=0 || seen[m.CharID] || m.Level<=0 || m.Cycle<0 || m.Cycle>5 { return 0,errors.New("invalid member") }; seen[m.CharID]=true; if m.CharID==recipient { own=m } }
 if own==nil { return 0,errors.New("not admitted") }; n:=int32(1)+own.Cycle
 for _,m:=range members { if m.CharID!=recipient { n++; if own.Level-m.Level>=10 { n++ } } }; return n,nil
}
type RiftMember struct { CharID CharID; Level int32; PresentMS int64; LastSeenMS int64; Departed, Eligible, Claimed bool }
type RiftMonster struct { Class int32; Dead bool }
type RiftGlobe struct { ID EntityID; Pos Pos; Value int32 }
type RiftRun struct {
 RunID string; Revision int64; Phase RiftPhase; Tier int32; Payer CharID
 CreatedMS,StartedMS,CompletedMS,LastMS,PenaltyMS int64
 Progress int32; Timely bool; Reason string
 Members map[CharID]*RiftMember
 Monsters map[EntityID]*RiftMonster
 Globes map[EntityID]RiftGlobe
}
func NewRiftRun(id string,tier int32,payer CharID, roster []TrialKeyMember,now int64)(*RiftRun,error){
 if id=="" || tier<1 || tier>RiftMaxTier {return nil,errors.New("invalid run")}; if _,err:=TrialKeyReward(payer,roster);err!=nil{return nil,err}
 r:=&RiftRun{RunID:id,Revision:1,Phase:RiftReserved,Tier:tier,Payer:payer,CreatedMS:now,LastMS:now,Members:map[CharID]*RiftMember{},Monsters:map[EntityID]*RiftMonster{},Globes:map[EntityID]RiftGlobe{}}
 for _,m:=range roster{r.Members[m.CharID]=&RiftMember{CharID:m.CharID,Level:m.Level,LastSeenMS:now}}; return r,nil
}
func(r *RiftRun)Clone()*RiftRun{b,_:=json.Marshal(r);var out RiftRun;_ =json.Unmarshal(b,&out);return &out}
func(r *RiftRun)Terminal()bool{return r.Phase==RiftCompleted||r.Phase==RiftClosed||r.Phase==RiftFailed||r.Phase==RiftAborted}
func(r *RiftRun)Admitted(id CharID)bool{m:=r.Members[id];return m!=nil&&!m.Departed}
func(r *RiftRun)Activate(now int64)bool{if r.Phase!=RiftReserved{return false};r.Phase=RiftActive;r.StartedMS=now;r.LastMS=now;r.Revision++;return true}
func(r *RiftRun)ObserveClock(now int64,present map[CharID]bool){
 if r.Terminal(){return};delta:=now-r.LastMS;if delta<0{delta=0};if delta>1000{delta=1000};r.LastMS=now
 for id,m:=range r.Members{if present[id]&&!m.Departed{m.LastSeenMS=now;if r.Phase!=RiftReserved{m.PresentMS+=delta}}}
 if r.Phase!=RiftReserved&&now-r.StartedMS>=RiftHardMS{r.Phase=RiftFailed;r.Reason="timeout";r.Revision++}
}
func(r *RiftRun)RegisterMonster(id EntityID,class int32)bool{if id==0||class<0||class>3||r.Monsters[id]!=nil{return false};r.Monsters[id]=&RiftMonster{Class:class};return true}
// RecordKill returns required orb count. Progress only comes from registered,
// first authoritative deaths; guardian completion requires the boss phase.
func(r *RiftRun)RecordKill(id EntityID,now int64)(int,bool){
 if r.Terminal()||r.Phase==RiftReserved{return 0,false};r.ObserveClock(now,nil);if r.Terminal(){return 0,false};m:=r.Monsters[id];if m==nil||m.Dead{return 0,false};m.Dead=true;r.Revision++
 if m.Class==3 {if r.Phase!=RiftBoss{return 0,false};r.Phase=RiftCompleted;r.CompletedMS=now;r.Timely=now-r.StartedMS+r.PenaltyMS<=RiftTimedMS
 elapsed:=now-r.StartedMS;if elapsed<1{elapsed=1};for _,p:=range r.Members{p.Eligible=!p.Departed&&now-p.LastSeenMS<=RiftReconnectMS&&p.PresentMS*2>=elapsed};return 0,true}
 if r.Phase!=RiftActive{return 0,false};switch m.Class{case 0:r.Progress+=RiftNormalProgress;case 1:return 1,true;case 2:return 4,true};r.advance();return 0,true
}
func(r *RiftRun)advance(){if r.Progress>=RiftProgressMax{r.Progress=RiftProgressMax;r.Phase=RiftBoss;r.Globes=map[EntityID]RiftGlobe{}}}
func(r *RiftRun)CollectGlobe(id EntityID,who CharID)bool{if r.Phase!=RiftActive||!r.Admitted(who){return false};g,ok:=r.Globes[id];if !ok{return false};delete(r.Globes,id);r.Progress+=g.Value;r.advance();r.Revision++;return true}
func(r *RiftRun)RecordDeath(id CharID)bool{if !r.Admitted(id)||r.Terminal()||r.Phase==RiftReserved{return false};r.PenaltyMS+=RiftDeathPenaltyMS;r.Revision++;return true}
func(r *RiftRun)Leave(id CharID)bool{m:=r.Members[id];if m==nil||m.Departed{return false};if !r.Terminal(){m.Departed=true;m.Eligible=false;all:=true;for _,p:=range r.Members{all=all&&p.Departed};if all{r.Phase=RiftAborted;r.Reason="abandoned"}};r.Revision++;return true}
func(r *RiftRun)MarkClaimed(id CharID)bool{m:=r.Members[id];if (r.Phase!=RiftCompleted&&r.Phase!=RiftClosed)||m==nil||!m.Eligible||m.Claimed{return false};m.Claimed=true;r.Revision++;return true}
func RiftScaledStat(base int32,tier int32,party int,class int32,hp bool)int32{if base<=0{return base};v:=float64(base);rate:=1.04;if hp{rate=1.08;v*=1+0.6*float64(party-1)};v*=math.Pow(rate,float64(tier-1));if hp{switch class{case 1:v*=4;case 2:v*=8;case 3:v*=40}}else{switch class{case 1:v*=1.2;case 2:v*=1.4;case 3:v*=1.5}};if v>math.MaxInt32{return math.MaxInt32};return int32(math.Ceil(v))}
type RiftRewardPlan struct{Equipment,Rune,Dust,UniqueChance,UnlockTier int32}
func PlanRiftRewards(tier int32,timely bool)RiftRewardPlan{p:=RiftRewardPlan{Equipment:1,Rune:1,Dust:6+(tier-1)/5,UniqueChance:20+2*tier,UnlockTier:tier};if p.UniqueChance>60{p.UniqueChance=60};if timely{p.Equipment++;p.Rune++;p.Dust+=4;p.UnlockTier++;if p.UnlockTier>RiftMaxTier{p.UnlockTier=RiftMaxTier}};return p}
