#version 410

in vec4 vertColor;
in vec2 vertUV0;
in vec3 fragPos;

out vec4 fragColor;

uniform sampler2D diffTex;

void main()
{
    vec4 texColor = texture(diffTex, vertUV0);
    // if (texColor.r == 0)
    // {
    //     fragColor = vec4(0,1,0,0.25);
    // }
    // else
    {
        fragColor = vec4(vertColor.rgb, texColor.r);
    }
} 
