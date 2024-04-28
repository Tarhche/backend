<template>
    <div>
        <dashboardNavbar />
        <slot />
        <Footer />
    </div>
</template>

<script setup>
import "~/assets/scss/dashboard/main.scss"

useHead({
  titleTemplate: (title) => title ? `داشبورد | ${ title }` : "داشبورد"
})
</script>
