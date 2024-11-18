"use client";
import {useState} from "react";
import {Stack, Tooltip, ActionIcon, Modal} from "@mantine/core";
import {UserAvatar} from "@/components/user-avatar";
import {FilesExplorer} from "@/components/files-explorer";
import {IconPencil} from "@tabler/icons-react";

type Props = {
  defaultValue?: string;
  username?: string;
  inputName?: string;
};

export function UserAvatarInput({
  inputName = "avatar",
  username,
  defaultValue,
}: Props) {
  const [showFileExplorer, setShowFileExplorer] = useState(false);
  const [selectedFileId, setSelectedFileId] = useState("");
  const avatarSrc = selectedFileId ? selectedFileId : defaultValue;

  return (
    <>
      <Stack align="center" gap={"xs"}>
        <UserAvatar email={username} src={avatarSrc} width={100} height={100} />
        <Tooltip label="تغییر آواتار" withArrow>
          <ActionIcon
            color="dimmed"
            variant="transparent"
            onClick={() => {
              setShowFileExplorer(true);
            }}
          >
            <IconPencil />
          </ActionIcon>
        </Tooltip>
      </Stack>
      <input name={inputName} value={selectedFileId} hidden readOnly />
      <Modal
        size={"xl"}
        opened={showFileExplorer}
        keepMounted={false}
        withCloseButton={false}
        centered
        onClose={() => {
          setShowFileExplorer(false);
        }}
      >
        <FilesExplorer
          onSelect={(fileId) => {
            setShowFileExplorer(false);
            setSelectedFileId(fileId);
          }}
        />
      </Modal>
    </>
  );
}
