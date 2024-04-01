<script lang="ts" setup>
if (useCookie("jwt")) {
  const cookie = useState('cookie' , () => useCookie("jwt"))
}
</script>

<template>
  <div>
    <NuxtLayout>
      <NuxtPage />
    </NuxtLayout>
  </div>
</template>
