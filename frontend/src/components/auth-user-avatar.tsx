"use client";
import Image from "next/image";
import {useState} from "react";
import {Avatar, Skeleton, useMantineTheme} from "@mantine/core";
import BoringAvatar from "boring-avatars";
import {useInit} from "@/hooks/data/init";
import {FILES_PUBLIC_URL} from "@/constants/envs";
import classes from "./auth-user-avatar.module.css";

type Props = {
  width?: number;
  height?: number;
};

export function AuthUserAvatar({width = 45, height = 45}: Props) {
  const {data, isLoading} = useInit();
  const theme = useMantineTheme();
  const colors = Object.values(theme.colors).map((c) => c[6]);
  const [hasImageFailed, setHasImageFailed] = useState(false);
  if (isLoading) {
    return <Skeleton circle width={width} height={height} />;
  }

  if (data?.status === "authenticated") {
    const {avatar, name, email} = data.profile;
    if (hasImageFailed || avatar === undefined) {
      return (
        <Avatar src={null} w={width} h={height}>
          <BoringAvatar
            variant="beam"
            name={email}
            size={width}
            colors={colors}
          />
        </Avatar>
      );
    }
    return (
      <Image
        src={`${FILES_PUBLIC_URL}/${avatar}`}
        alt={name}
        width={width}
        height={height}
        className={classes.avatar}
        onError={() => setHasImageFailed(true)}
      />
    );
  }

  return null;
}
