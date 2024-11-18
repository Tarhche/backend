"use client";
import Image from "next/image";
import {useState} from "react";
import {Avatar, useMantineTheme} from "@mantine/core";
import BoringAvatar from "boring-avatars";
import {FILES_PUBLIC_URL} from "@/constants/envs";
import classes from "./user-avatar.module.css";

type Props = {
  src?: string;
  email?: string;
  width?: number;
  height?: number;
};

export function UserAvatar({src, email, width = 45, height = 45}: Props) {
  const theme = useMantineTheme();
  const colors = Object.values(theme.colors).map((c) => c[5]);
  const [hasImageFailed, setHasImageFailed] = useState(false);
  const avatarSize = width === height ? width : Math.min(width, height);

  if (src === undefined || hasImageFailed) {
    if (email !== undefined) {
      return (
        <Avatar src={null} w={avatarSize} h={avatarSize}>
          <BoringAvatar
            variant="beam"
            name={email}
            size={avatarSize}
            colors={colors}
          />
        </Avatar>
      );
    }
    return <Avatar src={null} w={avatarSize} h={avatarSize} />;
  }

  return (
    <Image
      src={`${FILES_PUBLIC_URL}/${src}`}
      alt="user avatar"
      width={avatarSize}
      height={avatarSize}
      style={{
        minWidth: avatarSize,
        minHeight: avatarSize,
      }}
      className={classes.avatar}
      priority
      onError={() => setHasImageFailed(true)}
    />
  );
}
