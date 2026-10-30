; The Windows installer's own steps (tauri.windows.conf.json, installerHooks).
;
; The laptop agent runs from berth-cli.exe (or berth-agent.exe) in the
; install folder, and Windows won't let a running program be overwritten: an
; update stopped at "Error opening file for writing" until someone chose
; Retry. It will rename one, though. So each is moved aside before the new
; files go in, and the agent runs on from its old copy until the app, started
; again by the update, restarts it from the new one, once its work under way
; is done (berth agent restart --if-stale), as on a Mac. Nothing is stopped
; here and no console opens. Copies moved aside go at the next update, or
; with the app.

!macro BERTH_MOVE_ASIDE NAME
  Delete "$INSTDIR\${NAME}.old"
  Delete "$INSTDIR\${NAME}.old2"
  Rename "$INSTDIR\${NAME}" "$INSTDIR\${NAME}.old"
  ; .old still held by a run from the update before: the other name.
  IfFileExists "$INSTDIR\${NAME}" 0 +2
    Rename "$INSTDIR\${NAME}" "$INSTDIR\${NAME}.old2"
!macroend

!macro NSIS_HOOK_PREINSTALL
  !insertmacro BERTH_MOVE_ASIDE "berth-cli.exe"
  !insertmacro BERTH_MOVE_ASIDE "berth-agent.exe"
!macroend

!macro NSIS_HOOK_POSTUNINSTALL
  Delete "$INSTDIR\berth-cli.exe.old"
  Delete "$INSTDIR\berth-cli.exe.old2"
  Delete "$INSTDIR\berth-agent.exe.old"
  Delete "$INSTDIR\berth-agent.exe.old2"
!macroend
