; The Windows installer's own steps (tauri.windows.conf.json, installerHooks).
;
; The laptop agent runs from berth-cli.exe (or berth-agent.exe) in the
; install folder, and Windows won't let a running program be overwritten: an
; update stopped at "Error opening file for writing" until someone chose
; Retry. So the agent running from here is stopped before the files go in,
; and started again after, as the Mac app's restarts it after an update.
; A first install starts none: the app asks.

Var BerthAgentStopped

!macro NSIS_HOOK_PREINSTALL
  ; Only this install's agent, found by its path; prints how many it stopped.
  nsExec::ExecToStack `powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "$$p = @(Get-Process berth-cli,berth-agent -ErrorAction SilentlyContinue | Where-Object { $$_.Path -like '$INSTDIR\*' }); $$p | Stop-Process -Force; Start-Sleep -Milliseconds 500; Write-Output $$p.Count"`
  Pop $0
  Pop $BerthAgentStopped
!macroend

!macro NSIS_HOOK_POSTINSTALL
  StrCpy $0 $BerthAgentStopped 1
  StrCmp $0 "" +3
  StrCmp $0 "0" +2
  Exec '"$INSTDIR\berth-agent.exe" agent start'
!macroend
