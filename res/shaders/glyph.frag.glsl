#version 410

// uniform float ambientStrength = 0.1;
// uniform vec3 ambientLightColor = vec3(1, 1, 1);

// uniform vec3 lightPos1;
// uniform vec3 lightColor1;

uniform sampler2D diffTex;

in vec4 vertColor;
// in vec3 vertNormal;
in vec2 vertUV0;
in vec3 fragPos;

out vec4 fragColor;

void main()
{
    // vec3 lightDir = normalize(lightPos1 - fragPos);  
    // float diffStrength = max(0.0, dot(normalize(vertNormal), lightDir));

    // vec3 finalAmbientColor = ambientLightColor * ambientStrength;
    vec4 texColor = texture(diffTex, vertUV0);
    fragColor = vec4(vertColor.rgb, texColor.r*texColor.a);
    // fragColor = vec4(texColor.rgb * vertColor * (finalAmbientColor + diffStrength*lightColor1) , texColor.a);
    // Out_Color = vec4(Frag_Color.rgb, Frag_Color.a * texture(Texture, Frag_UV.st).r);
} 
