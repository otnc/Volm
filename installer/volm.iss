; Copyright (C) 2026 otoneko.
; SPDX-License-Identifier: GPL-3.0-only
;
; The installer of Volm. Build Volm.exe with "go tool mygo build -skip-installer" first, then from the repository root:
;
;   bash scripts/build-installer.sh 1.2.3
;
; which runs ISCC with /DAppVersion=1.2.3 and writes dist/Volm-Setup-1.2.3.exe.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

[Setup]
; AppId names the installed app to Inno Setup, which updates an installation of the same AppId in place, and the uninstall entry (Volm_is1). Never change it after a release.
AppId=Volm
AppName=Volm
AppVersion={#AppVersion}
AppVerName=Volm {#AppVersion}
AppPublisher=otoneko.
AppPublisherURL=https://github.com/otnc/Volm
AppSupportURL=https://github.com/otnc/Volm/issues
AppUpdatesURL=https://github.com/otnc/Volm/releases
AppCopyright=Copyright (C) 2026 otoneko.
VersionInfoVersion={#AppVersion}
DefaultDirName={localappdata}\Programs\Volm
DefaultGroupName=Volm
DisableProgramGroupPage=yes
DisableDirPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
LicenseFile=..\LICENSE
SetupIconFile=..\assets\volm.ico
UninstallDisplayIcon={app}\Volm.exe
UninstallDisplayName=Volm
OutputDir=..\dist
OutputBaseFilename=Volm-Setup-{#AppVersion}
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
; Volm holds this mutex while it runs. The [Code] below asks it to quit first; this is the fallback that tells the user.
AppMutex=VolmRunning
CloseApplications=yes
RestartApplications=no

[Languages]
Name: "japanese"; MessagesFile: "compiler:Languages\Japanese.isl"

[Tasks]
Name: "startup"; Description: "Windows の起動時に Volm を起動する"; GroupDescription: "そのほかの設定:"
Name: "desktopicon"; Description: "デスクトップにショートカットを作る"; GroupDescription: "そのほかの設定:"; Flags: unchecked

[Files]
Source: "..\build\windows-amd64\Volm.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"; Flags: ignoreversion
Source: "..\THIRD_PARTY_NOTICES.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.md"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\Volm"; Filename: "{app}\Volm.exe"
Name: "{autodesktop}\Volm"; Filename: "{app}\Volm.exe"; Tasks: desktopicon

[Registry]
; The same Run entry as MyGo's SetOpenAtLogin writes, so that the switch in the settings window shows it: the value is named after mygo.json's identifier, and the command carries MyGo's login argument.
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "io.github.otnc.volm"; ValueData: """{app}\Volm.exe"" --mygo-opened-at-login"; Tasks: startup; Flags: uninsdeletevalue

[Run]
Filename: "{app}\Volm.exe"; Description: "Volm を起動する"; Flags: postinstall nowait skipifsilent
; A silent install is an update from the settings window: start the new Volm without asking.
Filename: "{app}\Volm.exe"; Flags: nowait; Check: WizardSilent

[UninstallDelete]
Type: filesandordirs; Name: "{app}"

[Code]
const
  EVENT_MODIFY_STATE = $0002;
  SYNCHRONIZE = $00100000;
  QuitEventName = 'Local\VolmQuit';
  RunningMutexName = 'VolmRunning';

function OpenEvent(dwDesiredAccess: Cardinal; bInheritHandle: Boolean; lpName: String): THandle;
  external 'OpenEventW@kernel32.dll stdcall';
function SetEvent(hEvent: THandle): Boolean;
  external 'SetEvent@kernel32.dll stdcall';
function CloseHandle(hObject: THandle): Boolean;
  external 'CloseHandle@kernel32.dll stdcall';

{ Asks a running Volm to quit, as its settings window does, and waits up to five seconds for it to go. }
procedure QuitRunningVolm();
var
  Event: THandle;
  Waited: Integer;
begin
  Event := OpenEvent(EVENT_MODIFY_STATE, False, QuitEventName);
  if Event = 0 then
    Exit;
  SetEvent(Event);
  CloseHandle(Event);
  Waited := 0;
  while CheckForMutexes(RunningMutexName) and (Waited < 5000) do
  begin
    Sleep(100);
    Waited := Waited + 100;
  end;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  QuitRunningVolm();
  Result := '';
end;

function InitializeUninstall(): Boolean;
begin
  QuitRunningVolm();
  Result := True;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  DataDir: String;
begin
  if CurUninstallStep <> usPostUninstall then
    Exit;
  DataDir := ExpandConstant('{userappdata}\Volm');
  if not DirExists(DataDir) then
    Exit;
  if UninstallSilent() then
    Exit;
  if MsgBox('設定とログ (' + DataDir + ') も削除しますか？' + #13#10 + '「いいえ」を選ぶと、次にインストールしたときに今の設定がそのまま使われます。', mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
    DelTree(DataDir, True, True, True);
end;
