From RecoveryRefinement.Goose Require Import base.

Definition UseSlice  : proc unit :=
  s <- Data.newSlice byte 1;
  s1 <- Data.sliceAppendSlice s s;
  FS.atomicCreate "file" s1.

Definition UseMap  : proc unit :=
  m <- Data.newMap (slice.t byte);
  _ <- Data.mapAlter m 1 (fun _ => Some (slice.nil _));
  let! (x, ok) <- Data.mapGet m 2;
  if ok
  then Ret tt
  else Data.mapAlter m 3 (fun _ => Some x).

Definition UsePtr  : proc unit :=
  p <- Data.newPtr uint64;
  _ <- Data.writePtr p 1;
  x <- Data.readPtr p;
  Data.writePtr p x.

Definition Empty  : proc unit :=
  Ret tt.

Definition EmptyReturn  : proc unit :=
  Ret tt.

Module allTheLiterals.
  Record t := mk {
    int: uint64;
    s: string;
    b: bool;
  }.
  Global Instance t_zero : HasGoZero t := mk (zeroValue _) (zeroValue _) (zeroValue _).
End allTheLiterals.

Definition normalLiterals  : proc allTheLiterals.t :=
  Ret {| allTheLiterals.int := 0;
         allTheLiterals.s := "foo";
         allTheLiterals.b := true; |}.

Definition specialLiterals  : proc allTheLiterals.t :=
  Ret {| allTheLiterals.int := 4096;
         allTheLiterals.s := "";
         allTheLiterals.b := false; |}.

Definition oddLiterals  : proc allTheLiterals.t :=
  Ret {| allTheLiterals.int := 5;
         allTheLiterals.s := "backquote string";
         allTheLiterals.b := false; |}.

Definition DoSomeLocking (l:LockRef) : proc unit :=
  _ <- Data.lockAcquire l Writer;
  _ <- Data.lockRelease l Writer;
  _ <- Data.lockAcquire l Reader;
  _ <- Data.lockAcquire l Reader;
  _ <- Data.lockRelease l Reader;
  Data.lockRelease l Reader.

Definition MakeLock  : proc unit :=
  l <- Data.newLock;
  DoSomeLocking l.