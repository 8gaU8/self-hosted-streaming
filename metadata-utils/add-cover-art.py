import argparse
import subprocess
from pathlib import Path
from typing import Optional


def _is_image(file: Path) -> bool:
    return file.is_file() and file.suffix.lower() in [".jpg", ".jpeg", ".png"]


def search_cover_art(album_dir: Path) -> Optional[Path]:
    # strategy1: Search for cover art in the album directory
    for file in album_dir.iterdir():
        print(f"Checking file: {file.name}")
        if _is_image(file):
            return file

    # strategy2: Search for cover art in the current working directory
    cwd = Path.cwd()
    for file in cwd.iterdir():
        print(f"Checking file in cwd: {file.name}")
        if _is_image(file) and file.stem.lower() == album_dir.name.lower():
            return file
    return None


def build_ffpmeg_command_m4a(
    audio_file: Path, cover_path: Path, audio_dst: Path
) -> list[str]:
    command = [
        "ffmpeg",
        "-i",
        str(audio_file),
        "-i",
        str(cover_path),
        "-map",
        "0:a:0",
        "-map",
        "1:v:0",
        "-filter:v",
        "scale=w=500:h=500,format=yuvj420p",
        "-c:a",
        "copy",
        "-c:v",
        "mjpeg",
        "-disposition:v:0",
        "attached_pic",
        "-f",
        "ipod",
        "-movflags",
        "+faststart",
        str(audio_dst),
    ]
    return command


def build_ffpmeg_command_mp3(
    audio_file: Path, cover_path: Path, audio_dst: Path
) -> list[str]:
    command = [
        "ffmpeg",
        "-i",
        str(audio_file),
        "-i",
        str(cover_path),
        "-map",
        "0:a",
        "-map",
        "1:v",
        "-c",
        "copy",
        "-disposition:1",
        "attached_pic",
        "-id3v2_version",
        "3",
        "-metadata:s:v",
        'title="Album cover"',
        "-metadata:s:v",
        'comment="Cover (front)"',
        str(audio_dst),
    ]
    return command


def add_cover_art_to_audio(audio_file: Path, cover_path: Path, replace: bool):
    tempral_audio_file = audio_file.with_suffix(".temp" + audio_file.suffix)

    if audio_file.suffix.lower() == ".m4a":
        command = build_ffpmeg_command_m4a(audio_file, cover_path, tempral_audio_file)
    elif audio_file.suffix.lower() == ".mp3":
        command = build_ffpmeg_command_mp3(audio_file, cover_path, tempral_audio_file)
    else:
        raise ValueError(f"Unsupported audio file format: {audio_file.suffix}")

    result = subprocess.run(command, check=True)
    result.check_returncode()

    if replace:
        audio_file.unlink()
        tempral_audio_file.rename(audio_file)


def main():
    argparser = argparse.ArgumentParser(description="Add cover art to audio files.")
    argparser.add_argument("album_dir", type=Path)
    argparser.add_argument("--cover", type=Path, required=False, default=None)
    argparser.add_argument(
        "--no_replace",
        action="store_true",
        help="Replace the original audio files with the new ones.",
    )
    args = argparser.parse_args()

    album_dir = args.album_dir
    replace = not args.no_replace

    # search cover art
    cover_path = args.cover
    if cover_path is None:
        cover_path = search_cover_art(album_dir)
        if cover_path is None:
            print("No cover art found in the album directory.")
            return

    # Process each audio file in the album directory
    for audio_file in album_dir.iterdir():
        if audio_file.is_file() and audio_file.suffix.lower() in [
            ".mp3",
            ".m4a",
            ".flac",
        ]:
            print(f"Adding cover art to {audio_file.name}...")
            add_cover_art_to_audio(audio_file, cover_path, replace)
            print(f"Cover art added to {audio_file.name}.")


if __name__ == "__main__":
    main()

