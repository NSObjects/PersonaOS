package domain

import (
    "errors"
    "testing"
    "time"
)

func TestNewCharacterValidation(t *testing.T) {
    for _,tc:=range []struct{
        name,code,display,persona string
        wantErr bool
    }{
        {"valid","shinchan","蜡笔小新","活泼外向",false},
        {"invalid code","!","角色","人设",true},
        {"missing name","shinchan","","人设",true},
        {"missing persona","shinchan","角色","",true},
    }{
        t.Run(tc.name,func(t *testing.T){
            got,err:=NewCharacter(0,tc.code,tc.display,tc.persona,"",time.Time{},time.Time{})
            if (err!=nil)!=tc.wantErr {t.Fatalf("NewCharacter() error = %v, wantErr=%v; value=%+v",err,tc.wantErr,got)}
            if tc.wantErr&&!errors.Is(err,ErrInvalidCharacter){t.Fatalf("NewCharacter() error = %v, want ErrInvalidCharacter",err)}
        })
    }
}
func TestDeviceTypeSharedRoleIsAllowed(t *testing.T){
    a,err:=NewDeviceType(0,"A3","基础版","",7,false,true,true,time.Time{},time.Time{})
    if err!=nil{t.Fatal(err)}
    b,err:=NewDeviceType(0,"A4","视觉版","",7,true,true,true,time.Time{},time.Time{})
    if err!=nil{t.Fatal(err)}
    if a.CharacterID!=b.CharacterID||a.HasCamera||!b.HasCamera{
        t.Fatalf("device types = %+v and %+v, want same character and different camera capabilities",a,b)
    }
}
