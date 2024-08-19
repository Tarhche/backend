<script setup lang="ts">
const url = 'https://caspian14.cdn.asset.aparat.com/aparat-video/d60078eeb60aa84d16bbfd03e6f9114360412550-480p.mp4?wmsAuthSign=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbiI6ImM3ZTVkZGFjN2FhM2U4YjMwMjNkNzAzMDJlZTEzM2Q0IiwiZXhwIjoxNzI0MTExODY5LCJpc3MiOiJTYWJhIElkZWEgR1NJRyJ9.IWLkIlBqV6lm1sRvQfYWHPe9DXDyJurveXOgIoQdX0M'
</script>

<template>
  <div class="container" style="height: 500px;">
    <video-player :video="url"/>
  </div>
</template>
