From RecoveryRefinement.Goose Require Import base.

Module partialFile.
  Record t {model:GoModel} := mk {
    off: uint64;
    data: slice.t byte;
  }.
  Arguments mk {model}.
  Global Instance t_zero {model:GoModel} : HasGoZero t := mk (zeroValue _) (zeroValue _).
End partialFile.

Definition getUserDir {model:GoModel} (user:uint64) : proc string :=
  Ret ("user" ++ uint64_to_string user).

Definition SpoolDir : string := "spool".

Definition NumUsers : uint64 := 100.

Definition readMessage {model:GoModel} (userDir:string) (name:string) : proc (slice.t byte) :=
  f <- FS.open userDir name;
  fileContents <- Data.newPtr (slice.t byte);
  _ <- Loop (fun pf =>
        buf <- FS.readAt f pf.(partialFile.off) 4096;
        newData <- Data.sliceAppendSlice pf.(partialFile.data) buf;
        if compare_to (slice.length buf) 4096 Lt
        then
          _ <- Data.writePtr fileContents newData;
          LoopRet tt
        else
          Continue {| partialFile.off := pf.(partialFile.off);
                      partialFile.data := newData; |}) {| partialFile.off := 0;
           partialFile.data := slice.nil _; |};
  fileData <- Data.readPtr fileContents;
  Ret fileData.

Module Message.
  Record t {model:GoModel} := mk {
    Id: string;
    Contents: slice.t byte;
  }.
  Arguments mk {model}.
  Global Instance t_zero {model:GoModel} : HasGoZero t := mk (zeroValue _) (zeroValue _).
End Message.

(* Pickup reads all stored messages and acquires a per-user lock. *)
Definition Pickup {model:GoModel} (user:uint64) : proc (slice.t Message.t) :=
  ls <- Globals.getX;
  l <- Data.sliceRead ls user;
  _ <- Data.lockAcquire l Writer;
  userDir <- getUserDir user;
  names <- FS.list userDir;
  messages <- Data.newPtr (slice.t Message.t);
  initMessages <- Data.newSlice Message.t 0;
  _ <- Data.writePtr messages initMessages;
  _ <- Loop (fun i =>
        if i == slice.length names
        then LoopRet tt
        else
          name <- Data.sliceRead names i;
          msg <- readMessage userDir name;
          oldMessages <- Data.readPtr messages;
          newMessages <- Data.sliceAppend oldMessages {| Message.Id := name;
             Message.Contents := msg; |};
          _ <- Data.writePtr messages newMessages;
          Continue (i + 1)) 0;
  msgs <- Data.readPtr messages;
  Ret msgs.

Definition createTmp {model:GoModel} : proc (File * string) :=
  initID <- Data.randomUint64;
  finalFile <- Data.newPtr File;
  finalName <- Data.newPtr string;
  _ <- Loop (fun id =>
        let fname := uint64_to_string id in
        let! (f, ok) <- FS.create SpoolDir fname;
        if ok
        then
          _ <- Data.writePtr finalFile f;
          _ <- Data.writePtr finalName fname;
          LoopRet tt
        else
          newID <- Data.randomUint64;
          Continue newID) initID;
  f <- Data.readPtr finalFile;
  name <- Data.readPtr finalName;
  Ret (f, name).

Definition writeTmp {model:GoModel} (data:slice.t byte) : proc string :=
  let! (f, name) <- createTmp;
  _ <- Loop (fun buf =>
        if compare_to (slice.length buf) 4096 Lt
        then
          _ <- FS.append f buf;
          LoopRet tt
        else
          _ <- FS.append f (slice.take 4096 buf);
          Continue (slice.skip 4096 buf)) data;
  _ <- FS.close f;
  Ret name.

(* Deliver stores a new message.
   Does not require holding the per-user pickup/delete lock. *)
Definition Deliver {model:GoModel} (user:uint64) (msg:slice.t byte) : proc unit :=
  userDir <- getUserDir user;
  tmpName <- writeTmp msg;
  initID <- Data.randomUint64;
  _ <- Loop (fun id =>
        ok <- FS.link SpoolDir tmpName userDir ("msg" ++ uint64_to_string id);
        if ok
        then LoopRet tt
        else
          newID <- Data.randomUint64;
          Continue newID) initID;
  FS.delete SpoolDir tmpName.

(* Delete deletes a message for the current user.
   Requires the per-user lock, acquired with pickup. *)
Definition Delete {model:GoModel} (user:uint64) (msgID:string) : proc unit :=
  userDir <- getUserDir user;
  FS.delete userDir msgID.

(* Unlock releases the lock for the current user. *)
Definition Unlock {model:GoModel} (user:uint64) : proc unit :=
  locks <- Globals.getX;
  l <- Data.sliceRead locks user;
  Data.lockRelease l Writer.

Definition initLocks {model:GoModel} : proc unit :=
  locks <- Data.newPtr (slice.t LockRef);
  _ <- Loop (fun i =>
        if i == NumUsers
        then LoopRet tt
        else
          oldLocks <- Data.readPtr locks;
          l <- Data.newLock;
          newLocks <- Data.sliceAppend oldLocks l;
          _ <- Data.writePtr locks newLocks;
          Continue (i + 1)) 0;
  finalLocks <- Data.readPtr locks;
  Globals.setX finalLocks.

Definition Recover {model:GoModel} : proc unit :=
  _ <- initLocks;
  spooled <- FS.list SpoolDir;
  Loop (fun i =>
        if i == slice.length spooled
        then LoopRet tt
        else
          name <- Data.sliceRead spooled i;
          _ <- FS.delete SpoolDir name;
          Continue (i + 1)) 0.