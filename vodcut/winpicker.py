"""Open the native Windows file chooser from the local review server."""
import ctypes
from ctypes import wintypes

class OpenFileName(ctypes.Structure):
    _fields_ = [
        ("lStructSize", wintypes.DWORD), ("hwndOwner", wintypes.HWND),
        ("hInstance", wintypes.HINSTANCE), ("lpstrFilter", ctypes.c_void_p),
        ("lpstrCustomFilter", ctypes.c_void_p), ("nMaxCustFilter", wintypes.DWORD),
        ("nFilterIndex", wintypes.DWORD), ("lpstrFile", ctypes.c_void_p),
        ("nMaxFile", wintypes.DWORD), ("lpstrFileTitle", ctypes.c_void_p),
        ("nMaxFileTitle", wintypes.DWORD), ("lpstrInitialDir", wintypes.LPCWSTR),
        ("lpstrTitle", wintypes.LPCWSTR), ("Flags", wintypes.DWORD),
        ("nFileOffset", wintypes.WORD), ("nFileExtension", wintypes.WORD),
        ("lpstrDefExt", wintypes.LPCWSTR), ("lCustData", ctypes.c_ssize_t),
        ("lpfnHook", ctypes.c_void_p), ("lpTemplateName", wintypes.LPCWSTR),
        ("pvReserved", ctypes.c_void_p), ("dwReserved", wintypes.DWORD),
        ("FlagsEx", wintypes.DWORD),
    ]


def pick_file(kind: str) -> str:
    patterns = ("MP4, MKV, MOV", "*.mp4;*.mkv;*.mov") if kind == "video" else ("CSV, JSON", "*.csv;*.json")
    filters = ctypes.create_unicode_buffer("\0".join(patterns) + "\0\0")
    path = ctypes.create_unicode_buffer(32768)
    data = OpenFileName()
    data.lStructSize = ctypes.sizeof(data)
    data.lpstrFilter = ctypes.addressof(filters)
    data.nFilterIndex = 1
    data.lpstrFile = ctypes.addressof(path)
    data.nMaxFile = len(path)
    data.lpstrTitle = "VODCut Studio"
    # Explorer dialog with an existing file, an existing directory and an unchanged current directory.
    data.Flags = 0x80000 | 0x1000 | 0x800 | 0x8
    dialog = ctypes.windll.comdlg32
    dialog.GetOpenFileNameW.argtypes = [ctypes.POINTER(OpenFileName)]
    if dialog.GetOpenFileNameW(ctypes.byref(data)):
        return path.value
    if error := dialog.CommDlgExtendedError():
        raise OSError(f"File dialog error 0x{error:x}")
    return ""
