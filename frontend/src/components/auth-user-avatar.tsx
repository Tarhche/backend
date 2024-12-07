"use client";
import {Skeleton} from "@mantine/core";
import {UserAvatar} from "./user-avatar";
import {useInit} from "@/hooks/data/init";

type Props = {
  width?: number;
  height?: number;
};

export function AuthUserAvatar({width = 45, height = 45}: Props) {
  const {data, isLoading} = useInit();
  if (isLoading) {
    return <Skeleton circle width={width} height={height} />;
  }

  if (data?.status === "authenticated") {
    const {avatar, email} = data.profile;
    return (
      <UserAvatar email={email} src={avatar} width={width} height={height} />
    );
  }

  return null;
}
